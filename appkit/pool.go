package appkit

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	soothe "github.com/mirasoth/soothe-client-go"
)

// ErrPoolExhausted is returned when no free connection slot is available.
var ErrPoolExhausted = errors.New("appkit: connection pool exhausted")

// PoolConfig configures a ConnectionPool. Zero values are replaced with
// defaults by NewConnectionPool.
type PoolConfig struct {
	PoolSize            int
	QueryTimeout        time.Duration
	ConnectionTimeout   time.Duration
	MaxIdleTime         time.Duration
	HealthCheckInterval time.Duration
}

// DefaultPoolConfig returns conservative pool defaults.
func DefaultPoolConfig() *PoolConfig {
	return &PoolConfig{
		PoolSize:            1000,
		QueryTimeout:        30 * time.Minute,
		ConnectionTimeout:   30 * time.Second,
		MaxIdleTime:         10 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
	}
}

// pooledConn is one connection slot in the pool.
type pooledConn struct {
	slotID       int
	client       ManagedClient
	eventCh      <-chan interface{}
	streamCancel context.CancelFunc
	readerLive   atomic.Bool
	appKey       string
	loopID       string
	workspaceID  string
	lastUsed     time.Time
	mu           sync.RWMutex
}

// clientDisconnectNotified reports whether the underlying client signalled a drop.
func (c *pooledConn) clientDisconnectNotified() bool {
	select {
	case <-c.client.Disconnected():
		return true
	default:
		return false
	}
}

func (c *pooledConn) isConnected() bool {
	return c.client != nil && c.client.IsConnected() && !c.clientDisconnectNotified()
}

// eventStreamLive reports whether ReceiveMessages is still feeding eventCh.
func (c *pooledConn) eventStreamLive() bool {
	c.mu.RLock()
	ch := c.eventCh
	c.mu.RUnlock()
	return ch != nil && c.readerLive.Load()
}

// ConnectionPool manages a pool of daemon connections, one active per AppKey.
// It reuses an active connection when still live, otherwise bootstraps a fresh
// loop (loop_new + subscribe) or reattaches an existing one (loop_reattach +
// subscribe + ReattachAndProbe). Persistence of AppKey↔loop mappings is
// abstracted behind LoopSessionStore.
type ConnectionPool struct {
	url         string
	cfg         PoolConfig
	scfg        *soothe.Config
	factory     ClientFactory
	bootstrap   BootstrapFunc
	store       LoopSessionStore
	pool        chan *pooledConn
	activeSlots map[string]*pooledConn
	registry    map[int]string // slotID → appKey
	mu          sync.RWMutex
	nextSlotID  int
}

// NewConnectionPool constructs a pool for daemonURL. Nil cfg/scfg/factory
// fall back to defaults. The pool is pre-seeded with cfg.PoolSize idle slots.
func NewConnectionPool(daemonURL string, store LoopSessionStore, cfg *PoolConfig, scfg *soothe.Config, factory ClientFactory) *ConnectionPool {
	if cfg == nil {
		cfg = DefaultPoolConfig()
	}
	if scfg == nil {
		scfg = soothe.DefaultConfig()
	}
	if factory == nil {
		factory = DefaultClientFactory()
	}
	if cfg.PoolSize <= 0 {
		cfg.PoolSize = DefaultPoolConfig().PoolSize
	}
	p := &ConnectionPool{
		url:         daemonURL,
		cfg:         *cfg,
		scfg:        scfg,
		factory:     factory,
		bootstrap:   DefaultBootstrapFunc(),
		store:       store,
		pool:        make(chan *pooledConn, cfg.PoolSize),
		activeSlots: make(map[string]*pooledConn),
		registry:    make(map[int]string),
		nextSlotID:  1,
	}
	for i := 0; i < cfg.PoolSize; i++ {
		p.pool <- p.newSlot()
	}
	return p
}

// WithClientFactory overrides the client factory.
func (p *ConnectionPool) WithClientFactory(f ClientFactory) *ConnectionPool {
	if f != nil {
		p.factory = f
	}
	return p
}

// WithBootstrap overrides the loop bootstrap function.
func (p *ConnectionPool) WithBootstrap(f BootstrapFunc) *ConnectionPool {
	if f != nil {
		p.bootstrap = f
	}
	return p
}

// Stats returns a snapshot of active and idle slot counts.
func (p *ConnectionPool) Stats() (active, idle int) {
	p.mu.RLock()
	active = len(p.activeSlots)
	p.mu.RUnlock()
	idle = len(p.pool)
	return active, idle
}

func (p *ConnectionPool) newSlot() *pooledConn {
	p.mu.Lock()
	id := p.nextSlotID
	p.nextSlotID++
	p.mu.Unlock()
	return &pooledConn{
		slotID: id,
		client: p.factory(p.url, p.scfg),
	}
}

// Acquire returns a live connection for appKey, reusing an active slot or
// bootstrapping/reattaching as needed. The caller must call Release when done
// with the connection (a turn completes or the session is reset).
func (p *ConnectionPool) Acquire(ctx context.Context, appKey, workspaceID, userID string) (*pooledConn, error) {
	// 1. Reuse active connection when still live.
	p.mu.RLock()
	existing := p.activeSlots[appKey]
	p.mu.RUnlock()

	if existing != nil && (existing.clientDisconnectNotified() || !existing.isConnected() || !existing.eventStreamLive()) {
		log.Printf("[appkit.ConnectionPool] previous connection for %s dropped (or event stream dead), releasing for fresh bootstrap", appKey)
		p.Release(appKey)
		existing = nil
	}
	if existing != nil && existing.isConnected() && existing.eventStreamLive() {
		existing.mu.Lock()
		idleTooLong := p.cfg.MaxIdleTime > 0 && !existing.lastUsed.IsZero() &&
			time.Since(existing.lastUsed) > p.cfg.MaxIdleTime
		existing.mu.Unlock()
		if idleTooLong {
			log.Printf("[appkit.ConnectionPool] session %s idle beyond %v, releasing", appKey, p.cfg.MaxIdleTime)
			p.Release(appKey)
		} else {
			existing.mu.Lock()
			existing.lastUsed = time.Now()
			existing.mu.Unlock()
			_ = p.store.UpdateLastUsed(ctx, appKey)
			return existing, nil
		}
	}

	// 2. Pull a slot from the pool.
	select {
	case conn := <-p.pool:
		p.mu.Lock()
		p.activeSlots[appKey] = conn
		p.registry[conn.slotID] = appKey
		p.mu.Unlock()

		loopID, hasLoop := p.loopIDFor(ctx, appKey)
		sessionType := p.sessionTypeFor(ctx, appKey)
		var err error
		if !hasLoop || loopID == "" {
			// Fresh bootstrap.
			if err = conn.client.Connect(ctx); err != nil {
				p.Release(appKey)
				return nil, fmt.Errorf("connect: %w", err)
			}
			loopID, err = p.bootstrapNew(ctx, conn, appKey, workspaceID, userID)
			if err != nil {
				p.Release(appKey)
				return nil, fmt.Errorf("bootstrap new loop: %w", err)
			}
			if cerr := p.store.CreateSession(ctx, workspaceID, appKey, loopID, sessionType); cerr != nil {
				log.Printf("[appkit.ConnectionPool] WARN: create session failed for %s: %v", appKey, cerr)
			}
		} else {
			// Reattach.
			if err = p.resumeAndReattach(ctx, conn, loopID); err != nil {
				log.Printf("[appkit.ConnectionPool] reattach failed for %s, bootstrapping fresh: %v", appKey, err)
				if err = conn.client.Connect(ctx); err != nil {
					p.Release(appKey)
					return nil, fmt.Errorf("connect after reattach fail: %w", err)
				}
				loopID, err = p.bootstrapNew(ctx, conn, appKey, workspaceID, userID)
				if err != nil {
					p.Release(appKey)
					return nil, fmt.Errorf("bootstrap after reattach fail: %w", err)
				}
				if cerr := p.store.CreateSession(ctx, workspaceID, appKey, loopID, sessionType); cerr != nil {
					log.Printf("[appkit.ConnectionPool] WARN: create session after bootstrap failed for %s: %v", appKey, cerr)
				}
			}
		}

		conn.mu.Lock()
		conn.appKey = appKey
		conn.loopID = loopID
		conn.workspaceID = workspaceID
		conn.lastUsed = time.Now()
		conn.mu.Unlock()

		_ = p.store.UpdateLastUsed(ctx, appKey)
		log.Printf("[appkit.ConnectionPool] acquired slot %d for %s (loop %s)", conn.slotID, appKey, loopID)
		return conn, nil

	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return nil, ErrPoolExhausted
	}
}

// Release tears down the connection for appKey and returns a fresh slot to
// the pool.
func (p *ConnectionPool) Release(appKey string) {
	p.mu.Lock()
	conn := p.activeSlots[appKey]
	if conn != nil {
		delete(p.activeSlots, appKey)
		delete(p.registry, conn.slotID)
	}
	p.mu.Unlock()
	if conn == nil {
		return
	}
	if conn.streamCancel != nil {
		conn.streamCancel()
		conn.streamCancel = nil
	}
	if conn.client != nil {
		_ = conn.client.Close()
	}
	log.Printf("[appkit.ConnectionPool] released slot %d for %s", conn.slotID, appKey)
	select {
	case p.pool <- p.newSlot():
	default:
		log.Printf("[appkit.ConnectionPool] WARN: pool full when returning slot for %s", appKey)
	}
}

// ResetSession tears down the connection for appKey (cancelling any query
// externally first) so the next Acquire bootstraps fresh. The store should
// archive the loop id so GetLoopIDForSession returns false next time.
func (p *ConnectionPool) ResetSession(appKey string) {
	p.Release(appKey)
	log.Printf("[appkit.ConnectionPool] reset session %s — next message will create new loop", appKey)
}

// Stop gracefully shuts down all active connections.
func (p *ConnectionPool) Stop() {
	p.mu.Lock()
	slots := make([]*pooledConn, 0, len(p.activeSlots))
	for sid, conn := range p.activeSlots {
		slots = append(slots, conn)
		delete(p.activeSlots, sid)
		delete(p.registry, conn.slotID)
	}
	p.mu.Unlock()
	for _, conn := range slots {
		if conn.streamCancel != nil {
			conn.streamCancel()
		}
		if conn.client != nil {
			_ = conn.client.Close()
		}
	}
	for {
		select {
		case conn := <-p.pool:
			if conn.client != nil {
				_ = conn.client.Close()
			}
		default:
			return
		}
	}
}

// bootstrapNew assumes the client is already connected; it runs the bootstrap
// function (loop_new + subscribe) and starts the event reader.
// AppKey is attached to ctx for product-specific BootstrapFunc overrides.
func (p *ConnectionPool) bootstrapNew(ctx context.Context, conn *pooledConn, appKey, workspaceID, userID string) (string, error) {
	bootCtx := ContextWithAppKey(ctx, appKey)
	loopID, err := p.bootstrap(bootCtx, conn.client, workspaceID, userID, p.scfg)
	if err != nil {
		return "", err
	}
	p.startReader(ctx, conn)
	return loopID, nil
}

// resumeAndReattach reconnects + reattaches an existing loop, then starts the reader.
func (p *ConnectionPool) resumeAndReattach(ctx context.Context, conn *pooledConn, loopID string) error {
	if err := conn.client.Connect(ctx); err != nil {
		return err
	}
	if err := conn.client.ReattachAndProbe(ctx, loopID); err != nil {
		return err
	}
	p.startReader(ctx, conn)
	return nil
}

// startReader launches a ReceiveMessages goroutine and stores the event channel.
// The reader must outlive a single Acquire/Execute request: TurnRunner cancels
// its per-turn context when the turn ends, but the pooled connection is reused
// for later turns on the same appKey. Detach cancellation (keep values) so
// Release/Stop remain the only ways to tear the stream down.
func (p *ConnectionPool) startReader(ctx context.Context, conn *pooledConn) {
	if conn.streamCancel != nil {
		conn.streamCancel()
		conn.streamCancel = nil
	}
	rctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	raw, err := conn.client.ReceiveMessages(rctx)
	if err != nil {
		cancel()
		conn.readerLive.Store(false)
		log.Printf("[appkit.ConnectionPool] ReceiveMessages failed for %s: %v", conn.appKey, err)
		return
	}
	out := make(chan interface{})
	conn.mu.Lock()
	conn.eventCh = out
	conn.streamCancel = cancel
	conn.readerLive.Store(true)
	conn.mu.Unlock()
	go func() {
		defer func() {
			conn.readerLive.Store(false)
			close(out)
		}()
		for msg := range raw {
			select {
			case out <- msg:
			case <-rctx.Done():
				return
			}
		}
	}()
}

func (p *ConnectionPool) loopIDFor(ctx context.Context, appKey string) (string, bool) {
	loopID, ok, err := p.store.GetLoopIDForSession(ctx, appKey)
	if err != nil || !ok {
		return "", false
	}
	loopID = strings.TrimSpace(loopID)
	// Placeholder rows (e.g. Triarch pending-<chat_id>) must bootstrap fresh.
	if loopID == "" || strings.HasPrefix(loopID, "pending-") {
		return "", false
	}
	return loopID, true
}

func (p *ConnectionPool) sessionTypeFor(ctx context.Context, appKey string) string {
	if p.store == nil {
		return ""
	}
	entry, err := p.store.GetSession(ctx, appKey)
	if err != nil || entry == nil {
		return ""
	}
	return strings.TrimSpace(entry.SessionType)
}

// getLoopID returns the stored loop id for the connection.
func (c *pooledConn) getLoopID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.loopID
}
