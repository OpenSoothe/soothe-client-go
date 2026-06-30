package appkit

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
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

// DefaultPoolConfig returns env-overridable defaults (mirrors triarch).
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
	sessionID    string
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

// ConnectionPool manages a pool of daemon connections, one active per session.
// It reuses an active connection when still live, otherwise bootstraps a fresh
// loop (loop_new + subscribe) or reattaches an existing one (loop_reattach +
// subscribe + ReattachAndProbe). Persistence of session↔loop mappings is
// abstracted behind SessionStore.
//
// It is the app-agnostic successor to triarch's SoothePoolManager connection
// mechanics (RFC-629 Layer 1).
type ConnectionPool struct {
	cfg         PoolConfig
	scfg        *soothe.Config
	factory     ClientFactory
	bootstrap   BootstrapFunc
	store       SessionStore
	pool        chan *pooledConn
	activeSlots map[string]*pooledConn
	registry    map[int]string // slotID → sessionID
	mu          sync.RWMutex
	nextSlotID  int
}

// NewConnectionPool constructs a pool. If cfg is nil, DefaultPoolConfig is
// used; if scfg is nil, soothe.DefaultConfig is used; nil factory/bootstrap
// fall back to the defaults.
func NewConnectionPool(cfg *PoolConfig, scfg *soothe.Config, factory ClientFactory, store SessionStore) *ConnectionPool {
	if cfg == nil {
		cfg = DefaultPoolConfig()
	}
	if scfg == nil {
		scfg = soothe.DefaultConfig()
	}
	if factory == nil {
		factory = DefaultClientFactory()
	}
	return &ConnectionPool{
		cfg:         *cfg,
		scfg:        scfg,
		factory:     factory,
		bootstrap:   DefaultBootstrapFunc(),
		store:       store,
		pool:        make(chan *pooledConn, cfg.PoolSize),
		activeSlots: make(map[string]*pooledConn),
		registry:    make(map[int]string),
	}
}

// WithBootstrap overrides the loop bootstrap function (useful for test fakes
// or apps that build *soothe.Client wrappers).
func (p *ConnectionPool) WithBootstrap(f BootstrapFunc) *ConnectionPool {
	if f != nil {
		p.bootstrap = f
	}
	return p
}

// Acquire returns a live connection for sessionID, reusing an active slot or
// bootstrapping/reattaching as needed. The caller must call Release when done
// with the connection (a turn completes or the session is reset).
func (p *ConnectionPool) Acquire(ctx context.Context, sessionID, workspaceID, userID string) (*pooledConn, error) {
	// 1. Reuse active connection when still live.
	p.mu.RLock()
	existing := p.activeSlots[sessionID]
	p.mu.RUnlock()

	if existing != nil && (existing.clientDisconnectNotified() || !existing.isConnected()) {
		log.Printf("[appkit.ConnectionPool] previous connection for %s dropped, releasing for fresh bootstrap", sessionID)
		p.Release(sessionID)
		existing = nil
	}
	if existing != nil && existing.isConnected() {
		existing.mu.Lock()
		existing.lastUsed = time.Now()
		existing.mu.Unlock()
		_ = p.store.UpdateLastUsed(sessionID)
		return existing, nil
	}

	// 2. Pull a slot from the pool.
	select {
	case conn := <-p.pool:
		p.mu.Lock()
		p.activeSlots[sessionID] = conn
		p.registry[conn.slotID] = sessionID
		p.mu.Unlock()

		loopID, hasLoop := p.loopIDFor(sessionID)
		var err error
		if !hasLoop || loopID == "" {
			// Fresh bootstrap.
			if err = conn.client.Connect(ctx); err != nil {
				p.Release(sessionID)
				return nil, fmt.Errorf("connect: %w", err)
			}
			loopID, err = p.bootstrapNew(ctx, conn, workspaceID, userID)
			if err != nil {
				p.Release(sessionID)
				return nil, fmt.Errorf("bootstrap new loop: %w", err)
			}
			if cerr := p.store.CreateSession(workspaceID, sessionID, loopID, ""); cerr != nil {
				log.Printf("[appkit.ConnectionPool] WARN: create session failed for %s: %v", sessionID, cerr)
			}
		} else {
			// Reattach.
			if err = p.resumeAndReattach(ctx, conn, loopID); err != nil {
				log.Printf("[appkit.ConnectionPool] reattach failed for %s, bootstrapping fresh: %v", sessionID, err)
				if err = conn.client.Connect(ctx); err != nil {
					p.Release(sessionID)
					return nil, fmt.Errorf("connect after reattach fail: %w", err)
				}
				loopID, err = p.bootstrapNew(ctx, conn, workspaceID, userID)
				if err != nil {
					p.Release(sessionID)
					return nil, fmt.Errorf("bootstrap after reattach fail: %w", err)
				}
				if cerr := p.store.CreateSession(workspaceID, sessionID, loopID, ""); cerr != nil {
					log.Printf("[appkit.ConnectionPool] WARN: create session after bootstrap failed for %s: %v", sessionID, cerr)
				}
			}
		}

		conn.mu.Lock()
		conn.sessionID = sessionID
		conn.loopID = loopID
		conn.workspaceID = workspaceID
		conn.lastUsed = time.Now()
		conn.mu.Unlock()

		_ = p.store.UpdateLastUsed(sessionID)
		log.Printf("[appkit.ConnectionPool] acquired slot %d for %s (loop %s)", conn.slotID, sessionID, loopID)
		return conn, nil

	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return nil, ErrPoolExhausted
	}
}

// Release tears down the connection for sessionID and returns the slot.
func (p *ConnectionPool) Release(sessionID string) {
	p.mu.Lock()
	conn := p.activeSlots[sessionID]
	if conn != nil {
		delete(p.activeSlots, sessionID)
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
		conn.client.Close()
	}
	conn.mu.Lock()
	conn.sessionID = ""
	conn.loopID = ""
	conn.eventCh = nil
	conn.mu.Unlock()
	p.pool <- conn
	log.Printf("[appkit.ConnectionPool] released slot %d for %s", conn.slotID, sessionID)
}

// ResetSession tears down the connection for sessionID (cancelling any query
// externally first) so the next Acquire bootstraps fresh. The store should
// archive the loop id so GetLoopIDForSession returns false next time.
func (p *ConnectionPool) ResetSession(sessionID string) {
	p.Release(sessionID)
	log.Printf("[appkit.ConnectionPool] reset session %s — next message will create new loop", sessionID)
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
			conn.client.Close()
		}
	}
}

// bootstrapNew assumes the client is already connected; it runs the bootstrap
// function (loop_new + subscribe) and starts the event reader.
func (p *ConnectionPool) bootstrapNew(ctx context.Context, conn *pooledConn, workspaceID, userID string) (string, error) {
	loopID, err := p.bootstrap(ctx, conn.client, workspaceID, userID, p.scfg)
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
func (p *ConnectionPool) startReader(ctx context.Context, conn *pooledConn) {
	rctx, cancel := context.WithCancel(ctx)
	ch, err := conn.client.ReceiveMessages(rctx)
	if err != nil {
		cancel()
		log.Printf("[appkit.ConnectionPool] ReceiveMessages failed for %s: %v", conn.sessionID, err)
		return
	}
	conn.mu.Lock()
	conn.eventCh = ch
	conn.streamCancel = cancel
	conn.mu.Unlock()
}

func (p *ConnectionPool) loopIDFor(sessionID string) (string, bool) {
	loopID, ok, err := p.store.GetLoopIDForSession(sessionID)
	if err != nil || !ok {
		return "", false
	}
	return loopID, true
}

// getLoopID returns the stored loop id for the connection.
func (c *pooledConn) getLoopID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.loopID
}
