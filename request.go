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

// CommandRequest sends a structured RPC command and waits for the response (RFC-404).
func (c *Client) CommandRequest(ctx context.Context, command, threadID string, params map[string]interface{}, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	payload := map[string]interface{}{
		"type":    "command_request",
		"command": command,
	}
	if threadID != "" {
		payload["thread_id"] = threadID
	}
	if params != nil {
		payload["params"] = params
	}
	return c.RequestResponse(ctx, payload, "command_response", timeout)
}

// ThreadStatus requests runtime status for a thread and waits for the response.
func (c *Client) ThreadStatus(ctx context.Context, threadID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":      "thread_status",
		"thread_id": threadID,
	}, "thread_status_response", timeout)
}

// ThreadStats requests thread execution statistics and waits for the response.
func (c *Client) ThreadStats(ctx context.Context, threadID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":      "thread_stats",
		"thread_id": threadID,
	}, "thread_stats_response", timeout)
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
func (c *Client) LoopSubscribe(ctx context.Context, loopID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":    "loop_subscribe",
		"loop_id": loopID,
	}, "loop_subscribe_response", timeout)
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
func (c *Client) LoopInput(ctx context.Context, loopID string, content map[string]interface{}, timeout time.Duration) (map[string]interface{}, error) {
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

// CommandClear clears thread history.
func (c *Client) CommandClear(ctx context.Context, threadID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.CommandRequest(ctx, "clear", threadID, nil, timeout)
}

// CommandExit stops thread and marks for exit.
func (c *Client) CommandExit(ctx context.Context, threadID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.CommandRequest(ctx, "exit", threadID, nil, timeout)
}

// CommandQuit stops thread and marks for exit (alias for exit).
func (c *Client) CommandQuit(ctx context.Context, threadID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.CommandRequest(ctx, "quit", threadID, nil, timeout)
}

// CommandDetach marks thread as detached (continues running).
func (c *Client) CommandDetach(ctx context.Context, threadID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.CommandRequest(ctx, "detach", threadID, nil, timeout)
}

// CommandCancel cancels running query.
func (c *Client) CommandCancel(ctx context.Context, threadID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.CommandRequest(ctx, "cancel", threadID, nil, timeout)
}

// CommandMemory queries memory stats.
func (c *Client) CommandMemory(ctx context.Context, threadID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.CommandRequest(ctx, "memory", threadID, nil, timeout)
}

// CommandPolicy queries policy profile.
func (c *Client) CommandPolicy(ctx context.Context, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.CommandRequest(ctx, "policy", "", nil, timeout)
}

// CommandHistory queries input history.
func (c *Client) CommandHistory(ctx context.Context, threadID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.CommandRequest(ctx, "history", threadID, nil, timeout)
}

// CommandConfig queries configuration.
func (c *Client) CommandConfig(ctx context.Context, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.CommandRequest(ctx, "config", "", nil, timeout)
}

// CommandReview queries conversation history.
func (c *Client) CommandReview(ctx context.Context, threadID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.CommandRequest(ctx, "review", threadID, nil, timeout)
}

// CommandPlan queries current plan.
func (c *Client) CommandPlan(ctx context.Context, threadID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.CommandRequest(ctx, "plan", threadID, nil, timeout)
}

// CommandAutopilotDashboard shows autopilot dashboard.
func (c *Client) CommandAutopilotDashboard(ctx context.Context, threadID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return c.CommandRequest(ctx, "autopilot_dashboard", threadID, nil, timeout)
}
