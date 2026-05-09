package soothe

import (
	"context"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Request-Response pattern (mirrors Python SDK request_response)
// ---------------------------------------------------------------------------

// RequestResponse sends a request payload with a request_id and waits for a
// response with a matching request_id and the expected response type.
// If payload does not already set a non-empty request_id, one is generated.
// Events not matching the request_id are skipped.
func (c *Client) RequestResponse(ctx context.Context, payload map[string]interface{}, responseType string, timeout time.Duration) (map[string]interface{}, error) {
	rid, _ := payload["request_id"].(string)
	if rid == "" {
		rid = NewRequestID()
		payload["request_id"] = rid
	}

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

// WaitForSubscriptionConfirmed waits for subscription confirmation matching loopID.
func (c *Client) WaitForSubscriptionConfirmed(loopID string, verbosity string, timeout time.Duration) error {
	_ = verbosity
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
		typ, _ := ev["type"].(string)
		if typ == "loop_subscribe_response" {
			if ok, _ := ev["success"].(bool); ok {
				if lid, _ := ev["loop_id"].(string); lid == loopID {
					return nil
				}
			}
			continue
		}
		if typ != "subscription_confirmed" {
			continue
		}
		if lid, _ := ev["loop_id"].(string); lid == loopID {
			return nil
		}
	}
}

// CommandRequest sends a structured RPC command and waits for the response (RFC-404).
func (c *Client) CommandRequest(ctx context.Context, command, loopID string, params map[string]interface{}, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	payload := map[string]interface{}{
		"type":    "command_request",
		"command": command,
	}
	if loopID != "" {
		payload["loop_id"] = loopID
	}
	if params != nil {
		payload["params"] = params
	}
	return c.RequestResponse(ctx, payload, "command_response", timeout)
}

// LoopList requests the loop list and waits for the response.
func (c *Client) LoopList(ctx context.Context, filter map[string]interface{}, limit int, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	payload := map[string]interface{}{
		"type": "loop_list",
	}
	if filter != nil {
		payload["filter"] = filter
	}
	if limit > 0 {
		payload["limit"] = limit
	}
	return c.RequestResponse(ctx, payload, "loop_list_response", timeout)
}

// LoopGet requests loop details and waits for the response.
func (c *Client) LoopGet(ctx context.Context, loopID string, verbose bool, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	payload := map[string]interface{}{
		"type":    "loop_get",
		"loop_id": loopID,
	}
	if verbose {
		payload["verbose"] = true
	}
	return c.RequestResponse(ctx, payload, "loop_get_response", timeout)
}

// LoopTree requests the checkpoint tree for a loop and waits for the response.
func (c *Client) LoopTree(ctx context.Context, loopID, format string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	payload := map[string]interface{}{
		"type":    "loop_tree",
		"loop_id": loopID,
	}
	if format != "" {
		payload["format"] = format
	}
	return c.RequestResponse(ctx, payload, "loop_tree_response", timeout)
}

// LoopPrune requests pruning of old branches and waits for the response.
func (c *Client) LoopPrune(ctx context.Context, loopID string, retentionDays int, dryRun bool, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	payload := map[string]interface{}{
		"type":    "loop_prune",
		"loop_id": loopID,
	}
	if retentionDays > 0 {
		payload["retention_days"] = retentionDays
	}
	if dryRun {
		payload["dry_run"] = true
	}
	return c.RequestResponse(ctx, payload, "loop_prune_response", timeout)
}

// LoopDelete requests loop deletion and waits for the response.
func (c *Client) LoopDelete(ctx context.Context, loopID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":    "loop_delete",
		"loop_id": loopID,
	}, "loop_delete_response", timeout)
}

// LoopReattach requests loop reattachment and waits for the response (RFC-411).
func (c *Client) LoopReattach(ctx context.Context, loopID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":    "loop_reattach",
		"loop_id": loopID,
	}, "loop_reattach_response", timeout)
}

// LoopSubscribe subscribes to loop events and waits for the response (RFC-503).
// Pass empty verbosity to omit the field (daemon default).
func (c *Client) LoopSubscribe(ctx context.Context, loopID string, verbosity string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	payload := map[string]interface{}{
		"type":    "loop_subscribe",
		"loop_id": loopID,
	}
	if verbosity != "" {
		payload["verbosity"] = verbosity
	}
	return c.RequestResponse(ctx, payload, "loop_subscribe_response", timeout)
}

// LoopDetach detaches from a loop and waits for the response (RFC-503).
func (c *Client) LoopDetach(ctx context.Context, loopID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":    "loop_detach",
		"loop_id": loopID,
	}, "loop_detach_response", timeout)
}

// LoopNew creates a new loop and waits for the response (RFC-503).
func (c *Client) LoopNew(ctx context.Context, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type": "loop_new",
	}, "loop_new_response", timeout)
}

// LoopInput sends input to a loop and waits for the response (RFC-503).
// Content is the user prompt text (same wire shape as Python send_loop_input).
func (c *Client) LoopInput(ctx context.Context, loopID string, content string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	payload := map[string]interface{}{
		"type":    "loop_input",
		"loop_id": loopID,
		"content": content,
	}
	return c.RequestResponse(ctx, payload, "loop_input_response", timeout)
}

// ---------------------------------------------------------------------------
// RPC Command convenience methods (RFC-404)
// These wrap CommandRequest for common daemon slash commands
// ---------------------------------------------------------------------------

// CommandClear clears loop conversation history.
func (c *Client) CommandClear(ctx context.Context, loopID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.CommandRequest(ctx, "clear", loopID, nil, timeout)
}

// CommandExit stops the loop and marks for exit.
func (c *Client) CommandExit(ctx context.Context, loopID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.CommandRequest(ctx, "exit", loopID, nil, timeout)
}

// CommandQuit stops the loop and marks for exit (alias for exit).
func (c *Client) CommandQuit(ctx context.Context, loopID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.CommandRequest(ctx, "quit", loopID, nil, timeout)
}

// CommandDetach marks the loop as detached (continues running).
func (c *Client) CommandDetach(ctx context.Context, loopID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.CommandRequest(ctx, "detach", loopID, nil, timeout)
}

// CommandCancel cancels running query.
func (c *Client) CommandCancel(ctx context.Context, loopID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.CommandRequest(ctx, "cancel", loopID, nil, timeout)
}

// CommandMemory queries memory stats.
func (c *Client) CommandMemory(ctx context.Context, loopID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.CommandRequest(ctx, "memory", loopID, nil, timeout)
}

// CommandPolicy queries policy profile.
func (c *Client) CommandPolicy(ctx context.Context, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.CommandRequest(ctx, "policy", "", nil, timeout)
}

// CommandHistory queries input history.
func (c *Client) CommandHistory(ctx context.Context, loopID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.CommandRequest(ctx, "history", loopID, nil, timeout)
}

// CommandConfig queries configuration.
func (c *Client) CommandConfig(ctx context.Context, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.CommandRequest(ctx, "config", "", nil, timeout)
}

// CommandReview queries conversation history.
func (c *Client) CommandReview(ctx context.Context, loopID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.CommandRequest(ctx, "review", loopID, nil, timeout)
}

// CommandPlan queries current plan.
func (c *Client) CommandPlan(ctx context.Context, loopID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.CommandRequest(ctx, "plan", loopID, nil, timeout)
}

// CommandAutopilotDashboard shows autopilot dashboard.
func (c *Client) CommandAutopilotDashboard(ctx context.Context, loopID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.CommandRequest(ctx, "autopilot_dashboard", loopID, nil, timeout)
}
