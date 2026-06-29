package soothe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
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
	// Protocol-1 handshake state (RFC-450 §8.2)
	handshakeComplete      bool
	negotiatedCapabilities map[string]struct{}
	protocolVersion        string
	readinessState         string
	heartbeatIntervalMs    int
}

// NewClient creates a new Soothe daemon WebSocket client.
func NewClient(url string, cfg *Config) *Client {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Client{url: url, config: cfg}
}

// NewClientWithHeartbeat creates a client with automatic heartbeat tracking enabled.
func NewClientWithHeartbeat(url string, cfg *Config) *Client {
	c := NewClient(url, cfg)
	c.EnableHeartbeatTracking()
	return c
}

// Connect dials the Soothe daemon WebSocket and completes the protocol-1
// connection_init/connection_ack handshake (RFC-450 §8.2). Returns an error
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
// readiness_state "ready" (RFC-450 §8.2). Discards the leading status frame.
func (c *Client) handshake(ctx context.Context) error {
	timeout := c.config.DaemonReadyTimeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if c.conn != nil {
		_ = c.conn.SetReadDeadline(time.Now().Add(timeout))
		defer c.conn.SetReadDeadline(time.Time{})
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

// Close shuts down the WebSocket connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.conn == nil {
		return nil
	}
	c.closed = true
	// Use WriteControl to send close frame — it's safe even if a
	// ReadMessage is in progress (uses its own write buffer).
	_ = c.conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(time.Second))
	c.conn.Close()
	c.conn = nil
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
	return c.conn.WriteMessage(websocket.TextMessage, payload)
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
	go func() {
		defer close(ch)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			// Check if closed before reading.
			c.mu.Lock()
			isClosed := c.closed || c.conn == nil
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
				_, rd, err := c.conn.ReadMessage()
				if err != nil {
					data = nil
					return
				}
				data = rd
			}()
			if data == nil {
				return
			}
			for _, frame := range SplitSootheWirePayload(data) {
				msg, err := DecodeMessage(frame)
				if err != nil || msg == nil {
					continue
				}
				for _, expanded := range ExpandWireMessages(msg) {
					// Intercept protocol-level ping/pong (RFC-450 §8.3).
					if c.isControlFrame(expanded) {
						continue
					}
					// Automatically process heartbeat events if tracking is enabled
					if tracker != nil {
						// Convert to map for heartbeat processing
						if m, ok := expanded.(map[string]interface{}); ok {
							tracker.ProcessHeartbeatEvent(m)
						}
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
			// Intercept protocol-level ping/pong (RFC-450 §8.3).
			if c.isControlFrame(expanded) {
				continue
			}
			// Convert typed messages to map for uniform handling
			b, err := json.Marshal(expanded)
			if err != nil {
				return nil, err
			}
			var m map[string]interface{}
			if err := json.Unmarshal(b, &m); err != nil {
				return nil, err
			}
			return m, nil
		}
	}
	return nil, nil
}
