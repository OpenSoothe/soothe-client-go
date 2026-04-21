package soothe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Client manages a WebSocket session with the Soothe daemon.
// It is NOT safe for concurrent use from multiple goroutines except where noted.
// After Close(), a new Client must be created to reconnect.
type Client struct {
	url    string
	config *Config
	conn   *websocket.Conn
	mu     sync.Mutex // guards conn writes
}

// NewClient creates a new Soothe daemon WebSocket client.
func NewClient(url string, cfg *Config) *Client {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Client{url: url, config: cfg}
}

// Connect dials the Soothe daemon WebSocket and completes the HTTP upgrade.
// WebSocket-level ping/pong is disabled per RFC-0013 (daemon uses application heartbeats).
func (c *Client) Connect(ctx context.Context) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	header := http.Header{}
	conn, _, err := dialer.DialContext(ctx, c.url, header)
	if err != nil {
		return fmt.Errorf("soothe dial: %w", err)
	}
	c.conn = conn
	return nil
}

// Close shuts down the WebSocket connection.
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	c.conn.Close()
	c.conn = nil
	return err
}

// IsConnected returns whether the client has an active WebSocket connection.
func (c *Client) IsConnected() bool {
	return c.conn != nil
}

// SendMessage serialises msg as JSON and sends it as a WebSocket text frame.
func (c *Client) SendMessage(ctx context.Context, msg interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("soothe: not connected")
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("soothe marshal: %w", err)
	}
	return c.conn.WriteMessage(websocket.TextMessage, payload)
}

// ReceiveMessages starts reading frames from the daemon and returns decoded
// messages on the returned channel. The channel is closed when the connection
// ends or the context is cancelled.
func (c *Client) ReceiveMessages(ctx context.Context) (<-chan interface{}, error) {
	if c.conn == nil {
		return nil, fmt.Errorf("soothe: not connected")
	}
	ch := make(chan interface{}, 100)
	go func() {
		defer close(ch)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			_, data, err := c.conn.ReadMessage()
			if err != nil {
				return
			}
			for _, frame := range SplitSootheWirePayload(data) {
				msg, err := DecodeMessage(frame)
				if err != nil || msg == nil {
					continue
				}
				select {
				case ch <- msg:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return ch, nil
}

// ReadEvent reads a single event from the daemon. Returns nil on connection close.
func (c *Client) ReadEvent() (map[string]interface{}, error) {
	if c.conn == nil {
		return nil, fmt.Errorf("soothe: not connected")
	}
	_, data, err := c.conn.ReadMessage()
	if err != nil {
		return nil, nil // connection closed
	}
	for _, frame := range SplitSootheWirePayload(data) {
		msg, err := DecodeMessage(frame)
		if err != nil || msg == nil {
			continue
		}
		// Convert typed messages to map for uniform handling
		b, err := json.Marshal(msg)
		if err != nil {
			return nil, err
		}
		var m map[string]interface{}
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		return m, nil
	}
	return nil, nil
}