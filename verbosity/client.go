package soothe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/mirasurf/lepton/soothe-client-go/config"
	"github.com/mirasurf/lepton/soothe-client-go/protocol"
)

// Client manages a WebSocket session with the Soothe daemon.
// It is NOT safe for concurrent use from multiple goroutines except where noted.
// After Close(), a new Client must be created to reconnect.
type Client struct {
	url    string
	config *config.Config
	conn   *websocket.Conn
	mu     sync.Mutex // guards conn writes
}

// NewClient creates a new Soothe daemon WebSocket client.
func NewClient(url string, cfg *config.Config) *Client {
	if cfg == nil {
		cfg = config.DefaultConfig()
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
			for _, frame := range protocol.SplitSootheWirePayload(data) {
				msg, err := protocol.DecodeMessage(frame)
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
	for _, frame := range protocol.SplitSootheWirePayload(data) {
		msg, err := protocol.DecodeMessage(frame)
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

// ---------------------------------------------------------------------------
// High-level API methods (mirroring Python SDK WebSocketClient)
// ---------------------------------------------------------------------------

// SendInput sends user input to the daemon.
func (c *Client) SendInput(ctx context.Context, text string, opts ...InputOption) error {
	o := &inputOptions{autonomous: false}
	for _, opt := range opts {
		opt(o)
	}
	payload := map[string]interface{}{
		"type":        "input",
		"text":        text,
		"autonomous":  o.autonomous,
	}
	if o.maxIterations != nil {
		payload["max_iterations"] = *o.maxIterations
	}
	if o.subagent != "" {
		payload["subagent"] = o.subagent
	}
	if o.interactive {
		payload["interactive"] = true
	}
	if o.model != "" {
		payload["model"] = o.model
	}
	if o.modelParams != nil {
		payload["model_params"] = o.modelParams
	}
	if o.threadID != "" {
		payload["thread_id"] = o.threadID
	}
	return c.SendMessage(ctx, payload)
}

// InputOption configures an input message.
type InputOption func(*inputOptions)

type inputOptions struct {
	threadID      string
	autonomous    bool
	maxIterations *int
	subagent      string
	interactive   bool
	model         string
	modelParams   map[string]interface{}
}

// WithThreadID sets the thread ID for the input message.
func WithThreadID(threadID string) InputOption {
	return func(o *inputOptions) { o.threadID = threadID }
}

// WithAutonomous enables autonomous mode.
func WithAutonomous(maxIterations *int) InputOption {
	return func(o *inputOptions) {
		o.autonomous = true
		o.maxIterations = maxIterations
	}
}

// WithSubagent routes the query to a specific subagent.
func WithSubagent(name string) InputOption {
	return func(o *inputOptions) { o.subagent = name }
}

// WithInteractive enables interactive mode.
func WithInteractive() InputOption {
	return func(o *inputOptions) { o.interactive = true }
}

// WithModel sets an optional provider:model override.
func WithModel(model string) InputOption {
	return func(o *inputOptions) { o.model = model }
}

// WithModelParams sets extra model parameters.
func WithModelParams(params map[string]interface{}) InputOption {
	return func(o *inputOptions) { o.modelParams = params }
}

// SendCommand sends a slash command to the daemon.
func (c *Client) SendCommand(ctx context.Context, cmd string) error {
	return c.SendMessage(ctx, protocol.CommandMessage{
		BaseMessage: protocol.BaseMessage{Type: "command"},
		Cmd:         cmd,
	})
}

// SendNewThread requests the daemon to start a new thread.
func (c *Client) SendNewThread(ctx context.Context, workspace string) error {
	return c.SendMessage(ctx, protocol.NewNewThreadMessage(workspace))
}

// SendResumeThread requests the daemon to resume a specific thread.
func (c *Client) SendResumeThread(ctx context.Context, threadID, workspace string) error {
	return c.SendMessage(ctx, protocol.NewResumeThreadMessage(threadID, workspace))
}

// SendSubscribeThread subscribes to events for a thread.
func (c *Client) SendSubscribeThread(ctx context.Context, threadID, verbosity string) error {
	return c.SendMessage(ctx, protocol.NewSubscribeThreadMessage(threadID, verbosity))
}

// SendDetach notifies the daemon that this client is detaching.
func (c *Client) SendDetach(ctx context.Context) error {
	return c.SendMessage(ctx, protocol.DetachMessage{
		BaseMessage: protocol.BaseMessage{Type: "detach"},
	})
}

// SendDaemonReady sends the daemon_ready handshake message.
func (c *Client) SendDaemonReady(ctx context.Context) error {
	return c.SendMessage(ctx, protocol.BaseMessage{Type: "daemon_ready"})
}

// SendDaemonStatus requests daemon status check.
func (c *Client) SendDaemonStatus(ctx context.Context, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = protocol.NewRequestID()
	}
	return c.SendMessage(ctx, protocol.DaemonStatusMessage{
		BaseMessage: protocol.BaseMessage{RequestID: rid, Type: "daemon_status"},
	})
}

// SendDaemonShutdown requests daemon shutdown.
func (c *Client) SendDaemonShutdown(ctx context.Context, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = protocol.NewRequestID()
	}
	return c.SendMessage(ctx, protocol.DaemonShutdownMessage{
		BaseMessage: protocol.BaseMessage{RequestID: rid, Type: "daemon_shutdown"},
	})
}

// SendConfigGet requests a config section from the daemon.
func (c *Client) SendConfigGet(ctx context.Context, section string, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = protocol.NewRequestID()
	}
	return c.SendMessage(ctx, protocol.ConfigGetMessage{
		BaseMessage: protocol.BaseMessage{RequestID: rid, Type: "config_get"},
		Section:     section,
	})
}

// SendThreadList requests the persisted thread list.
func (c *Client) SendThreadList(ctx context.Context, filter map[string]interface{}, includeStats bool, includeLastMessage bool, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = protocol.NewRequestID()
	}
	return c.SendMessage(ctx, protocol.ThreadListMessage{
		BaseMessage:        protocol.BaseMessage{RequestID: rid, Type: "thread_list"},
		Filter:             filter,
		IncludeStats:       includeStats,
		IncludeLastMessage: includeLastMessage,
	})
}

// SendThreadGet requests metadata for a specific thread.
func (c *Client) SendThreadGet(ctx context.Context, threadID string, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = protocol.NewRequestID()
	}
	return c.SendMessage(ctx, protocol.ThreadGetMessage{
		BaseMessage: protocol.BaseMessage{RequestID: rid, Type: "thread_get"},
		ThreadID:    threadID,
	})
}

// SendThreadMessages requests paginated thread messages.
func (c *Client) SendThreadMessages(ctx context.Context, threadID string, limit, offset int, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = protocol.NewRequestID()
	}
	return c.SendMessage(ctx, protocol.ThreadMessagesMessage{
		BaseMessage: protocol.BaseMessage{RequestID: rid, Type: "thread_messages"},
		ThreadID:    threadID,
		Limit:       limit,
		Offset:      offset,
	})
}

// SendThreadState requests raw checkpoint state for a thread.
func (c *Client) SendThreadState(ctx context.Context, threadID string, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = protocol.NewRequestID()
	}
	return c.SendMessage(ctx, protocol.ThreadStateMessage{
		BaseMessage: protocol.BaseMessage{RequestID: rid, Type: "thread_state"},
		ThreadID:    threadID,
	})
}

// SendThreadUpdateState persists partial state values for a thread.
func (c *Client) SendThreadUpdateState(ctx context.Context, threadID string, values map[string]interface{}, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = protocol.NewRequestID()
	}
	return c.SendMessage(ctx, protocol.ThreadUpdateStateMessage{
		BaseMessage: protocol.BaseMessage{RequestID: rid, Type: "thread_update_state"},
		ThreadID:    threadID,
		Values:      values,
	})
}

// SendThreadArchive requests thread archival.
func (c *Client) SendThreadArchive(ctx context.Context, threadID string, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = protocol.NewRequestID()
	}
	return c.SendMessage(ctx, protocol.ThreadArchiveMessage{
		BaseMessage: protocol.BaseMessage{RequestID: rid, Type: "thread_archive"},
		ThreadID:    threadID,
	})
}

// SendThreadDelete requests thread deletion.
func (c *Client) SendThreadDelete(ctx context.Context, threadID string, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = protocol.NewRequestID()
	}
	return c.SendMessage(ctx, protocol.ThreadDeleteMessage{
		BaseMessage: protocol.BaseMessage{RequestID: rid, Type: "thread_delete"},
		ThreadID:    threadID,
	})
}

// SendThreadCreate requests creation of a persisted thread (RFC-402).
func (c *Client) SendThreadCreate(ctx context.Context, initialMessage string, metadata map[string]interface{}, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = protocol.NewRequestID()
	}
	return c.SendMessage(ctx, protocol.ThreadCreateMessage{
		BaseMessage:     protocol.BaseMessage{RequestID: rid, Type: "thread_create"},
		InitialMessage:  initialMessage,
		Metadata:        metadata,
	})
}

// SendThreadArtifacts requests thread artifacts (RFC-402).
func (c *Client) SendThreadArtifacts(ctx context.Context, threadID string, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = protocol.NewRequestID()
	}
	return c.SendMessage(ctx, protocol.ThreadArtifactsMessage{
		BaseMessage: protocol.BaseMessage{RequestID: rid, Type: "thread_artifacts"},
		ThreadID:    threadID,
	})
}

// SendResumeInterrupts sends interactive continuation payload for a paused thread.
func (c *Client) SendResumeInterrupts(ctx context.Context, threadID string, resumePayload map[string]interface{}, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = protocol.NewRequestID()
	}
	return c.SendMessage(ctx, protocol.ResumeInterruptsMessage{
		BaseMessage:    protocol.BaseMessage{RequestID: rid, Type: "resume_interrupts"},
		ThreadID:       threadID,
		ResumePayload:  resumePayload,
	})
}

// SendSkillsList requests the skills catalog (RFC-400).
func (c *Client) SendSkillsList(ctx context.Context, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = protocol.NewRequestID()
	}
	return c.SendMessage(ctx, protocol.SkillsListMessage{
		BaseMessage: protocol.BaseMessage{RequestID: rid, Type: "skills_list"},
	})
}

// SendModelsList requests the models catalog (RFC-400).
func (c *Client) SendModelsList(ctx context.Context, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = protocol.NewRequestID()
	}
	return c.SendMessage(ctx, protocol.ModelsListMessage{
		BaseMessage: protocol.BaseMessage{RequestID: rid, Type: "models_list"},
	})
}

// SendInvokeSkill invokes a skill on the daemon (RFC-400).
func (c *Client) SendInvokeSkill(ctx context.Context, skill, args string, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = protocol.NewRequestID()
	}
	return c.SendMessage(ctx, protocol.InvokeSkillMessage{
		BaseMessage: protocol.BaseMessage{RequestID: rid, Type: "invoke_skill"},
		Skill:       skill,
		Args:        args,
	})
}

// ---------------------------------------------------------------------------
// Request-Response pattern (mirrors Python SDK request_response)
// ---------------------------------------------------------------------------

// RequestResponse sends a request payload with a unique request_id and waits
// for a response with a matching request_id and the expected response type.
// Events not matching the request_id are skipped.
func (c *Client) RequestResponse(ctx context.Context, payload map[string]interface{}, responseType string, timeout time.Duration) (map[string]interface{}, error) {
	rid := protocol.NewRequestID()
	payload["request_id"] = rid

	if err := c.SendMessage(ctx, payload); err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	// Set a read deadline on the underlying connection to prevent blocking forever
	if c.conn != nil {
		c.conn.SetReadDeadline(time.Now().Add(timeout))
		defer c.conn.SetReadDeadline(time.Time{}) // clear deadline
	}

	timeoutCh := time.After(timeout)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeoutCh:
			return nil, fmt.Errorf("timeout after %v waiting for %s", timeout, responseType)
		default:
		}

		ev, err := c.ReadEvent()
		if err != nil {
			return nil, fmt.Errorf("read event: %w", err)
		}
		if ev == nil {
			return nil, fmt.Errorf("connection closed waiting for %s", responseType)
		}

		if evRid, ok := ev["request_id"].(string); !ok || evRid != rid {
			continue
		}
		if typ, _ := ev["type"].(string); typ == "error" {
			msg, _ := ev["message"].(string)
			return nil, fmt.Errorf("daemon error: %s", msg)
		}
		if typ, _ := ev["type"].(string); typ == responseType {
			return ev, nil
		}
	}
}

// ---------------------------------------------------------------------------
// Convenience RPC methods (mirrors Python SDK helpers)
// ---------------------------------------------------------------------------

// ListSkills requests the skills catalog and waits for the response.
func (c *Client) ListSkills(ctx context.Context, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return c.RequestResponse(ctx, map[string]interface{}{"type": "skills_list"}, "skills_list_response", timeout)
}

// ListModels requests the models catalog and waits for the response.
func (c *Client) ListModels(ctx context.Context, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return c.RequestResponse(ctx, map[string]interface{}{"type": "models_list"}, "models_list_response", timeout)
}

// InvokeSkill resolves a skill on the daemon host and receives echo (RFC-400).
func (c *Client) InvokeSkill(ctx context.Context, skill, args string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":  "invoke_skill",
		"skill": skill,
		"args":  args,
	}, "invoke_skill_response", timeout)
}

// WaitForDaemonReady reads events until a daemon_ready with state == "ready".
func (c *Client) WaitForDaemonReady(timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if c.conn != nil {
		c.conn.SetReadDeadline(time.Now().Add(timeout))
		defer c.conn.SetReadDeadline(time.Time{})
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for {
		select {
		case <-deadline.C:
			return nil, fmt.Errorf("timeout after %v waiting for daemon_ready", timeout)
		default:
		}

		ev, err := c.ReadEvent()
		if err != nil {
			return nil, err
		}
		if ev == nil {
			return nil, fmt.Errorf("connection closed waiting for daemon_ready")
		}
		if typ, _ := ev["type"].(string); typ != "daemon_ready" {
			continue
		}
		if state, _ := ev["state"].(string); state == "ready" {
			return ev, nil
		}
		msg, _ := ev["message"].(string)
		if msg == "" {
			msg = fmt.Sprintf("daemon state is %v", ev["state"])
		}
		return nil, fmt.Errorf("daemon not ready: %s", msg)
	}
}

// WaitForSubscriptionConfirmed waits for a subscription_confirmed matching the thread_id.
func (c *Client) WaitForSubscriptionConfirmed(threadID string, verbosity string, timeout time.Duration) error {
	_ = verbosity // soothe-sdk logs a warning on mismatch; we only require thread_id
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if c.conn != nil {
		c.conn.SetReadDeadline(time.Now().Add(timeout))
		defer c.conn.SetReadDeadline(time.Time{})
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for {
		select {
		case <-deadline.C:
			return fmt.Errorf("timeout after %v waiting for subscription_confirmed", timeout)
		default:
		}

		ev, err := c.ReadEvent()
		if err != nil {
			return err
		}
		if ev == nil {
			return fmt.Errorf("connection closed waiting for subscription_confirmed")
		}
		if typ, _ := ev["type"].(string); typ != "subscription_confirmed" {
			continue
		}
		if tid, _ := ev["thread_id"].(string); tid == threadID {
			return nil
		}
	}
}
