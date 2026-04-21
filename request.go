package soothe

import (
	"context"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Request-Response pattern (mirrors Python SDK request_response)
// ---------------------------------------------------------------------------

// RequestResponse sends a request payload with a unique request_id and waits
// for a response with a matching request_id and the expected response type.
// Events not matching the request_id are skipped.
func (c *Client) RequestResponse(ctx context.Context, payload map[string]interface{}, responseType string, timeout time.Duration) (map[string]interface{}, error) {
	rid := NewRequestID()
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