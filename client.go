package soothe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// Client manages a WebSocket session with the Soothe daemon.
// It is NOT safe for concurrent use from multiple goroutines except where noted.
// After Close(), a new Client must be created to reconnect.
type Client struct {
	url              string
	config           *Config
	conn             *websocket.Conn
	mu               sync.Mutex // guards conn writes and closed flag
	closed           bool
	heartbeatTracker *HeartbeatTracker // optional heartbeat tracker for daemon health monitoring
	// Protocol-1 handshake state
	handshakeComplete      bool
	negotiatedCapabilities map[string]struct{}
	protocolVersion        string
	readinessState         string
	heartbeatIntervalMs    int
	// Mid-session drop signal. disconnCh is closed exactly once
	// when the connection drops; disconnCause carries clean vs unclean.
	disconnCh    chan DisconnectCause
	disconnOnce  sync.Once
	disconnCause DisconnectCause
	// Pending-request/subscription multiplexer.
	// Routes inbound frames by (type, id) instead of discarding non-matching events.
	mux *mux
	// readerActive is non-zero while a ReceiveMessages goroutine is reading
	// from the socket. When set, RequestResponse must NOT call ReadEvent
	// (gorilla/websocket forbids concurrent readers); it waits purely on the
	// mux channels for its response.
	readerActive atomic.Bool
}

// NewClient creates a new Soothe daemon WebSocket client.
func NewClient(url string, cfg *Config) *Client {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Client{
		url:       url,
		config:    cfg,
		disconnCh: make(chan DisconnectCause, 1),
		mux:       newMux(),
	}
}

// Disconnected returns a channel that is closed exactly once when the
// connection drops. The cause value readable before close distinguishes a
// clean disconnect notification (loops keep running server-side) from an
// unclean loss (read/write error or missed pong; in-flight queries are cancelled).
// Returns nil if the client was never connected.
func (c *Client) Disconnected() <-chan DisconnectCause {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.disconnCh
}

// DisconnectCauseValue returns the cause of the most recent drop, or zero
// if the connection has not dropped. Pair with Disconnected() for the signal.
func (c *Client) DisconnectCauseValue() DisconnectCause {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.disconnCause
}

// signalDisconnect delivers the disconnect cause to the channel once, then
// closes it. Safe to call from any goroutine; subsequent calls are no-ops.
func (c *Client) signalDisconnect(cause DisconnectCause) {
	c.disconnOnce.Do(func() {
		c.mu.Lock()
		c.disconnCause = cause
		ch := c.disconnCh
		c.mu.Unlock()
		// Send the cause (buffered cap 1) then close so readers both receive
		// the value and unblock. Closing a channel alone yields the zero value,
		// which would lose the clean/unclean distinction.
		select {
		case ch <- cause:
		default:
		}
		close(ch)
	})
}

// NewClientWithHeartbeat creates a client with automatic heartbeat tracking enabled.
func NewClientWithHeartbeat(url string, cfg *Config) *Client {
	c := NewClient(url, cfg)
	c.EnableHeartbeatTracking()
	return c
}

// Connect dials the Soothe daemon WebSocket and completes the protocol-1
// connection_init/connection_ack handshake. Returns an error
// if the daemon does not report readiness_state "ready" within the configured
// DaemonReadyTimeout.
func (c *Client) Connect(ctx context.Context) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	header := http.Header{}
	conn, _, err := dialer.DialContext(ctx, c.url, header)
	if err != nil {
		return fmt.Errorf("soothe dial: %w", err)
	}
	c.mu.Lock()
	c.conn = conn
	c.closed = false
	// Reset the drop signal and multiplexer for a fresh connection so
	// Reconnect() on the same Client starts with a clean slate.
	c.disconnCh = make(chan DisconnectCause, 1)
	c.disconnOnce = sync.Once{}
	c.disconnCause = 0
	c.mux = newMux()
	c.handshakeComplete = false
	c.mu.Unlock()

	// Perform the protocol-1 handshake. On failure, tear down the socket so a
	// reconnect does not reuse a half-open connection.
	if err := c.handshake(ctx); err != nil {
		c.mu.Lock()
		c.closed = true
		c.conn = nil
		c.mu.Unlock()
		_ = conn.Close()
		return err
	}
	return nil
}

// handshake sends connection_init and waits for connection_ack with
// readiness_state "ready". Discards the leading status frame.
func (c *Client) handshake(ctx context.Context) error {
	timeout := c.config.DaemonReadyTimeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if c.conn != nil {
		_ = c.conn.SetReadDeadline(time.Now().Add(timeout))
		defer func() { _ = c.conn.SetReadDeadline(time.Time{}) }()
	}

	sendInit := func() error {
		return c.SendMessage(ctx, NewConnectionInitEnvelope())
	}
	if err := sendInit(); err != nil {
		return fmt.Errorf("connection_init: %w", err)
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ev, err := c.ReadEvent()
		if err != nil {
			return fmt.Errorf("handshake read: %w", err)
		}
		if ev == nil {
			return fmt.Errorf("connection closed during handshake")
		}
		typ, _ := ev["type"].(string)
		// Discard the leading status frame; keep other frames for later readers.
		if typ == "status" {
			continue
		}
		// Handle Warn-severity error frames during the handshake window:
		// DAEMON_STARTING (-32001), DAEMON_BUSY (-32002),
		// DAEMON_DEGRADED (-32003) are retryable; other errors are fatal.
		if typ == "error" {
			errObj, _ := ev["error"].(map[string]interface{})
			code := -32603
			if ic, ok := errObj["code"].(float64); ok {
				code = int(ic)
			}
			if isRetryableWarnCode(code) {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(100 * time.Millisecond):
				}
				if err := sendInit(); err != nil {
					return fmt.Errorf("connection_init retry after warn: %w", err)
				}
				continue
			}
			msg, _ := errObj["message"].(string)
			return fmt.Errorf("daemon handshake error [%d]: %s", code, msg)
		}
		if typ != "connection_ack" {
			continue
		}
		result, _ := ev["result"].(map[string]interface{})
		state, _ := result["readiness_state"].(string)
		c.protocolVersion, _ = result["protocol_version"].(string)
		if c.protocolVersion == "" {
			c.protocolVersion = ProtoVersion
		}
		caps := map[string]struct{}{}
		if rawCaps, ok := result["capabilities"].([]interface{}); ok {
			for _, cap := range rawCaps {
				if s, ok := cap.(string); ok {
					caps[s] = struct{}{}
				}
			}
		}
		c.negotiatedCapabilities = caps
		if hb, ok := result["heartbeat_interval_ms"].(float64); ok {
			c.heartbeatIntervalMs = int(hb)
		}

		switch state {
		case "ready":
			c.handshakeComplete = true
			c.readinessState = state
			return nil
		case "incompatible":
			return fmt.Errorf("protocol version incompatible: daemon returned %s", c.protocolVersion)
		case "error":
			return fmt.Errorf("daemon startup failed")
		case "degraded":
			return fmt.Errorf("daemon is degraded")
		case "starting", "warming":
			// Bounded retry: re-send connection_init after a short sleep.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(50 * time.Millisecond):
			}
			if err := sendInit(); err != nil {
				return fmt.Errorf("connection_init retry: %w", err)
			}
			continue
		default:
			return fmt.Errorf("daemon state is %s", state)
		}
	}
	return fmt.Errorf("timeout after %v waiting for connection_ack", timeout)
}

// IsHandshakeComplete reports whether the protocol-1 handshake has completed.
func (c *Client) IsHandshakeComplete() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.handshakeComplete
}

// ReadinessState returns the daemon readiness_state from the connection_ack.
func (c *Client) ReadinessState() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.readinessState
}

// Close shuts down the WebSocket connection. It fires Disconnected() so
// consumers blocked on the signal unblock; the cause is unclean because an
// explicit teardown does not guarantee server-side loop continuity.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed || c.conn == nil {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()
	// Use WriteControl to send close frame — it's safe even if a
	// ReadMessage is in progress (uses its own write buffer).
	_ = conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(time.Second))
	_ = conn.Close()
	// Release any waiters blocked on Disconnected().
	c.signalDisconnect(DisconnectUnclean)
	return nil
}

// Reconnect re-dials the daemon and re-handshakes after a connection drop.
// It does not re-establish loop subscriptions; follow with
// ReattachAndProbe to resume a loop session. The caller should invoke this
// after Disconnected() fires. Reuses the same Client, resetting the disconnect
// signal and multiplexer.
func (c *Client) Reconnect(ctx context.Context) error {
	// Tear down any stale socket quietly (do not fire a redundant Disconnected
	// signal — the caller already observed the drop).
	c.mu.Lock()
	closed := c.closed
	conn := c.conn
	c.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
	_ = closed // closed flag is reset by Connect below

	// Dial + handshake. Connect resets disconnCh/disconnOnce/mux and performs
	// the connection_init/connection_ack handshake with readiness retry.
	return c.Connect(ctx)
}

// ReattachAndProbe resumes an existing loop after a reconnect: it issues
// loop_reattach, re-subscribes to loop_events, then runs a loop_get liveness
// probe to detect stale loops that accept the handshake but silently drop
// input. Returns *StaleLoopError when the probe fails; callers should fall
// back to a fresh loop_new bootstrap.
//
// Connection-level readiness is the handshake's readiness_state
// (+ daemon_status); loop_get is a loop-scoped probe only, not a readiness probe.
func (c *Client) ReattachAndProbe(ctx context.Context, loopID string) error {
	if loopID == "" {
		return fmt.Errorf("soothe: ReattachAndProbe requires a loop id")
	}

	// 1. loop_reattach: reconstruct event history and replay.
	reattachTimeout := c.config.LoopStatusTimeout
	if reattachTimeout <= 0 {
		reattachTimeout = 15 * time.Second
	}
	if _, err := c.RequestResponse(ctx, map[string]interface{}{
		"type":    "loop_reattach",
		"loop_id": loopID,
	}, "loop_reattach", reattachTimeout); err != nil {
		return fmt.Errorf("loop_reattach: %w", err)
	}

	// 2. Re-subscribe to the loop event stream (subscribe +
	//    method:"loop_events"). Confirmation arrives as a `next` frame.
	subTimeout := c.config.SubscriptionTimeout
	if subTimeout <= 0 {
		subTimeout = 10 * time.Second
	}
	if _, err := c.LoopSubscribe(ctx, loopID, c.config.VerbosityLevel, subTimeout); err != nil {
		return fmt.Errorf("loop_subscribe: %w", err)
	}

	// 3. loop_get liveness probe — side-effect-free read.
	//    A LOOP_NOT_FOUND (-32200) or timeout means the loop is stale: it
	//    accepted the reattach handshake but is not actually live.
	probeTimeout := c.config.ReattachProbeTimeout
	if probeTimeout <= 0 {
		probeTimeout = 5 * time.Second
	}
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	if _, err := c.LoopGet(probeCtx, loopID, false, probeTimeout); err != nil {
		var de *DaemonError
		if errors.As(err, &de) && de.Code == -32200 {
			return NewStaleLoopError(loopID, err)
		}
		// Timeout or other error during probe → treat as stale.
		return NewStaleLoopError(loopID, err)
	}
	return nil
}

// IsConnected returns whether the client has an active WebSocket connection.
func (c *Client) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil && !c.closed
}

// EnableHeartbeatTracking enables automatic heartbeat tracking for daemon health monitoring.
// The tracker will process heartbeat events as they are received.
func (c *Client) EnableHeartbeatTracking() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.heartbeatTracker == nil {
		c.heartbeatTracker = NewHeartbeatTracker()
	}
}

// EnableHeartbeatTrackingWithThreshold enables heartbeat tracking with a custom alive threshold.
func (c *Client) EnableHeartbeatTrackingWithThreshold(threshold time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.heartbeatTracker = NewHeartbeatTrackerWithThreshold(threshold)
}

// DisableHeartbeatTracking disables heartbeat tracking and clears the tracker.
func (c *Client) DisableHeartbeatTracking() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.heartbeatTracker = nil
}

// GetHeartbeatTracker returns the heartbeat tracker if enabled, or nil if disabled.
func (c *Client) GetHeartbeatTracker() *HeartbeatTracker {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.heartbeatTracker
}

// GetDaemonHealth returns the current daemon health status if heartbeat tracking is enabled.
// Returns nil if heartbeat tracking is not enabled.
func (c *Client) GetDaemonHealth() *DaemonHealth {
	c.mu.Lock()
	tracker := c.heartbeatTracker
	c.mu.Unlock()
	if tracker == nil {
		return nil
	}
	return tracker.GetHealth()
}

// IsDaemonAlive returns true if the daemon is considered alive based on heartbeat tracking.
// Returns false if heartbeat tracking is not enabled.
func (c *Client) IsDaemonAlive() bool {
	health := c.GetDaemonHealth()
	return health != nil && health.IsAlive
}

// SendMessage serialises msg as JSON and sends it as a WebSocket text frame.
func (c *Client) SendMessage(ctx context.Context, msg interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil || c.closed {
		return fmt.Errorf("soothe: not connected")
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("soothe: %w", ctx.Err())
	default:
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("soothe marshal: %w", err)
	}
	if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		// Write failure indicates a broken connection — signal an unclean drop
		// so consumers can reconnect. Idempotent if the read loop already fired.
		c.signalDisconnect(DisconnectUnclean)
		return err
	}
	return nil
}

// sendPong sends a protocol-1 pong frame in response to a daemon ping.
func (c *Client) sendPong() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil || c.closed {
		return
	}
	payload, err := json.Marshal(NewPongEnvelope())
	if err != nil {
		return
	}
	_ = c.conn.WriteMessage(websocket.TextMessage, payload)
}

// isControlFrame returns true if the decoded envelope is a ping/pong heartbeat
// frame and handles it (responds to ping with pong). Returns true when the
// frame was consumed and should NOT be forwarded to the application channel.
// A `disconnect` notification is a clean lifecycle frame: it is
// consumed here and fires a clean Disconnected() signal.
func (c *Client) isControlFrame(msg interface{}) bool {
	env, ok := msg.(Envelope)
	if !ok {
		return false
	}
	switch env.Type {
	case "ping":
		c.sendPong()
		return true
	case "pong":
		// Liveness tracked elsewhere; swallow.
		return true
	case "disconnect":
		// Graceful peer-initiated disconnect; loops keep running server-side.
		c.signalDisconnect(DisconnectClean)
		return true
	}
	return false
}

// ReceiveMessages starts reading frames from the daemon and returns decoded
// messages on the returned channel. The channel is closed when the connection
// ends or the context is cancelled.
// If heartbeat tracking is enabled, heartbeat events are automatically processed
// before being forwarded to the channel.
func (c *Client) ReceiveMessages(ctx context.Context) (<-chan interface{}, error) {
	c.mu.Lock()
	if c.conn == nil || c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("soothe: not connected")
	}
	tracker := c.heartbeatTracker
	c.mu.Unlock()
	ch := make(chan interface{}, 100)
	c.readerActive.Store(true)
	go func() {
		defer close(ch)
		defer c.readerActive.Store(false)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			// Check if closed before reading; snapshot the conn pointer under
			// the lock so a concurrent Close() niling c.conn cannot race with
			// the ReadMessage call below.
			c.mu.Lock()
			conn := c.conn
			isClosed := c.closed || conn == nil
			c.mu.Unlock()
			if isClosed {
				return
			}
			// Recover from any bufio panics caused by concurrent Close().
			var data []byte
			func() {
				defer func() {
					if r := recover(); r != nil {
						// Connection was closed during read — exit gracefully.
						data = nil
					}
				}()
				_, rd, err := conn.ReadMessage()
				if err != nil {
					data = nil
					return
				}
				data = rd
			}()
			if data == nil {
				// Abrupt read failure or panic: signal an unclean drop so
				// consumers (e.g. ConnectionPool) can reconnect + reattach.
				c.signalDisconnect(DisconnectUnclean)
				return
			}
			for _, frame := range SplitSootheWirePayload(data) {
				msg, err := DecodeMessage(frame)
				if err != nil || msg == nil {
					continue
				}
				for _, expanded := range ExpandWireMessages(msg) {
					// Intercept protocol-level ping/pong/disconnect.
					if c.isControlFrame(expanded) {
						continue
					}
					// Convert to map for uniform routing and heartbeat processing.
					var m map[string]interface{}
					if mv, ok := expanded.(map[string]interface{}); ok {
						m = mv
					} else {
						b, err := json.Marshal(expanded)
						if err != nil {
							continue
						}
						if err := json.Unmarshal(b, &m); err != nil {
							continue
						}
					}
					// Route solicited frames (response/error/next/complete with a
					// matching pending waiter) to their waiters; do not forward.
					if c.mux != nil && c.mux.route(m) {
						continue
					}
					// Automatically process heartbeat events if tracking is enabled
					if tracker != nil {
						tracker.ProcessHeartbeatEvent(m)
					}
					select {
					case ch <- expanded:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()
	return ch, nil
}

// ReadEvent reads a single event from the daemon. Returns nil, nil on normal connection close.
// After a timeout error, the connection enters a failed state and subsequent reads will panic.
// Callers should NOT retry ReadEvent after receiving a timeout error.
//
// ReadEvent consults the multiplexer: a frame matching another pending RPC or
// subscription waiter is routed to that waiter (and skipped), so concurrent
// in-flight RPCs on a shared reader no longer discard each other's responses.
func (c *Client) ReadEvent() (map[string]interface{}, error) {
	if c.conn == nil {
		return nil, fmt.Errorf("soothe: not connected")
	}

	// Recover from potential panic on failed websocket connection
	var data []byte
	var readErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				// Connection was in failed state - treat as connection closed
				readErr = fmt.Errorf("websocket connection failed: %v", r)
			}
		}()
		_, rd, err := c.conn.ReadMessage()
		data = rd
		readErr = err
	}()

	if readErr != nil {
		var netErr net.Error
		if errors.As(readErr, &netErr) && netErr.Timeout() {
			return nil, fmt.Errorf("websocket read timed out: %w", readErr)
		}
		return nil, nil // connection closed or failed
	}

	for _, frame := range SplitSootheWirePayload(data) {
		msg, err := DecodeMessage(frame)
		if err != nil || msg == nil {
			continue
		}
		for _, expanded := range ExpandWireMessages(msg) {
			// Intercept protocol-level ping/pong/disconnect.
			if c.isControlFrame(expanded) {
				continue
			}
			// Convert typed messages to map for uniform handling and routing.
			b, err := json.Marshal(expanded)
			if err != nil {
				return nil, err
			}
			var m map[string]interface{}
			if err := json.Unmarshal(b, &m); err != nil {
				return nil, err
			}
			// Route to a matching pending waiter if one exists; otherwise return
			// to the caller. This lets a single reader multiplex concurrent RPCs.
			if c.mux != nil && c.mux.route(m) {
				continue
			}
			return m, nil
		}
	}
	return nil, nil
}

// isRetryableWarnCode reports whether a daemon error code is a Warn-severity
// transient failure the client should retry after a delay.
func isRetryableWarnCode(code int) bool {
	switch code {
	case -32001, -32002, -32003: // DAEMON_STARTING, DAEMON_BUSY, DAEMON_DEGRADED
		return true
	}
	return false
}
