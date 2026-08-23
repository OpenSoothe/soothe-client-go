package soothe

import (
	"context"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Request-Response pattern (mirrors Python SDK request_response)
// ---------------------------------------------------------------------------

// RequestResponse sends a protocol-1 request envelope and waits for the
// matching response (or error) correlated by id.
//
// The payload's "type" field is used as the RPC method; all other fields are
// moved into the envelope "params". If the payload carries a "request_id" it
// is used as the correlation id, otherwise a new one is generated. The
// returned map is the response "result" object.
//
// RequestResponse is multiplexer-aware: it registers a
// pending call for its request id so that, if a concurrent ReceiveMessages
// reader is active, the response is routed to this caller instead of being
// discarded. When no concurrent reader is active, it reads synchronously via
// ReadEvent (which itself routes other waiters' frames to the mux).
func (c *Client) RequestResponse(ctx context.Context, payload map[string]interface{}, responseType string, timeout time.Duration) (map[string]interface{}, error) {
	method, _ := payload["type"].(string)
	if method == "" {
		return nil, fmt.Errorf("request payload missing 'type' (method)")
	}
	// Build params from the payload, dropping the type/request_id control fields.
	params := make(map[string]interface{}, len(payload))
	for k, v := range payload {
		if k == "type" || k == "request_id" {
			continue
		}
		params[k] = v
	}
	rid, _ := payload["request_id"].(string)
	env := NewRequestEnvelopeWithID(method, params, rid)
	rid = env.ID

	if err := c.SendMessage(ctx, env); err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	// Register a pending call so a concurrent reader can deliver the response.
	pc, unregister := c.mux.registerRPC(rid)
	defer unregister()

	// Set a read deadline on the underlying connection to prevent blocking forever
	if c.conn != nil {
		_ = c.conn.SetReadDeadline(time.Now().Add(timeout))
		defer func() { _ = c.conn.SetReadDeadline(time.Time{}) }() // clear deadline
	}

	timeoutCh := time.After(timeout)
	for {
		// If a ReceiveMessages reader is active, it owns the socket read; wait
		// purely on the mux channels (gorilla/websocket forbids concurrent
		// readers, so RequestResponse must not call ReadEvent here).
		if c.readerActive.Load() {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-timeoutCh:
				return nil, fmt.Errorf("timeout after %v waiting for %s", timeout, responseType)
			case result := <-pc.replyCh:
				return result, nil
			case err := <-pc.errCh:
				return nil, err
			}
		}

		// No concurrent reader: drain any already-routed response first.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeoutCh:
			return nil, fmt.Errorf("timeout after %v waiting for %s", timeout, responseType)
		case result := <-pc.replyCh:
			return result, nil
		case err := <-pc.errCh:
			return nil, err
		default:
		}

		// No concurrent reader delivered yet; read synchronously. ReadEvent
		// routes other waiters' frames to the mux and returns the first frame
		// that belongs to nobody — which may be our own response.
		ev, err := c.ReadEvent()
		if err != nil {
			return nil, fmt.Errorf("read event: %w", err)
		}
		if ev == nil {
			// Connection may have closed; check if a concurrent reader routed
			// our response before returning.
			select {
			case result := <-pc.replyCh:
				return result, nil
			case err := <-pc.errCh:
				return nil, err
			default:
				return nil, fmt.Errorf("connection closed waiting for %s", responseType)
			}
		}

		// Correlate by id. The daemon echoes the request id on response/error.
		evID, _ := ev["id"].(string)
		if evID != rid {
			// Not ours and not routed (unsolicited event) — re-route defensively
			// in case a waiter appeared between the ReadEvent and now.
			c.mux.route(ev)
			continue
		}
		typ, _ := ev["type"].(string)
		if typ == "error" {
			errObj, _ := ev["error"].(map[string]interface{})
			code := -32603
			if ic, ok := errObj["code"].(float64); ok {
				code = int(ic)
			}
			msg, _ := errObj["message"].(string)
			data, _ := errObj["data"].(map[string]interface{})
			return nil, &DaemonError{Code: code, Message: msg, Data: data}
		}
		if typ == "response" {
			result, _ := ev["result"].(map[string]interface{})
			if result == nil {
				result = ev
			}
			return result, nil
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

// InvokeSkill resolves a skill on the daemon host and receives echo.
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

// WaitForDaemonReady waits for the protocol-1 connection_ack handshake to
// report readiness_state "ready". The handshake is performed in Connect();
// this method returns immediately once the handshake is complete, or waits
// for an out-of-band connection_ack if Connect() did not complete it.
func (c *Client) WaitForDaemonReady(timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	// Fast path: handshake already completed during Connect().
	c.mu.Lock()
	ready := c.handshakeComplete
	state := c.readinessState
	c.mu.Unlock()
	if ready {
		return map[string]interface{}{"readiness_state": state}, nil
	}

	if c.conn != nil {
		_ = c.conn.SetReadDeadline(time.Now().Add(timeout))
		defer func() { _ = c.conn.SetReadDeadline(time.Time{}) }()
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for {
		select {
		case <-deadline.C:
			return nil, fmt.Errorf("timeout after %v waiting for connection_ack", timeout)
		default:
		}

		ev, err := c.ReadEvent()
		if err != nil {
			return nil, err
		}
		if ev == nil {
			return nil, fmt.Errorf("connection closed waiting for connection_ack")
		}
		if typ, _ := ev["type"].(string); typ != "connection_ack" {
			continue
		}
		result, _ := ev["result"].(map[string]interface{})
		state, _ := result["readiness_state"].(string)
		if state == "ready" {
			return ev, nil
		}
		return nil, fmt.Errorf("daemon not ready: state=%s", state)
	}
}

// CommandRequest sends a structured RPC command and waits for the response.
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

// LoopReattach requests loop reattachment and waits for the response.
func (c *Client) LoopReattach(ctx context.Context, loopID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":    "loop_reattach",
		"loop_id": loopID,
	}, "loop_reattach_response", timeout)
}

// LoopSubscribe subscribes to loop events and waits for the subscription
// confirmation `next` frame. Pass empty verbosity to omit the
// field (daemon default). Returns a map describing the subscription.
//
// Multiplexer-aware: when a ReceiveMessages reader is active, the confirmation
// `next` frame is routed via the mux subscription channel instead of calling
// ReadEvent (gorilla/websocket forbids concurrent readers).
func (c *Client) LoopSubscribe(ctx context.Context, loopID string, verbosity string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	params := map[string]interface{}{"loop_id": loopID}
	if verbosity != "" {
		params["verbosity"] = verbosity
	}
	env := NewSubscribeEnvelope("loop_events", params)
	if err := c.SendMessage(ctx, env); err != nil {
		return nil, fmt.Errorf("send subscribe: %w", err)
	}

	// Register a pending subscription so a concurrent reader can deliver the
	// confirmation next frame.
	streamCh, unsub := c.mux.registerSubscription(env.ID)
	defer unsub()

	if c.conn != nil {
		_ = c.conn.SetReadDeadline(time.Now().Add(timeout))
		defer func() { _ = c.conn.SetReadDeadline(time.Time{}) }()
	}
	timeoutCh := time.After(timeout)
	for {
		// If a ReceiveMessages reader is active, it owns the socket read; wait
		// purely on the mux channels (gorilla/websocket forbids concurrent
		// readers, so LoopSubscribe must not call ReadEvent here).
		if c.readerActive.Load() {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-timeoutCh:
				return nil, fmt.Errorf("timeout after %v waiting for loop_events confirmation", timeout)
			case frame := <-streamCh:
				ev, _ := frame.(map[string]interface{})
				if ev == nil {
					return nil, fmt.Errorf("connection closed waiting for loop_events confirmation")
				}
				typ, _ := ev["type"].(string)
				if typ == "error" {
					errObj, _ := ev["error"].(map[string]interface{})
					msg, _ := errObj["message"].(string)
					return nil, fmt.Errorf("daemon error: %s", msg)
				}
				payload, _ := ev["payload"].(map[string]interface{})
				return payload, nil
			}
		}

		// No concurrent reader: drain any already-routed frame first.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeoutCh:
			return nil, fmt.Errorf("timeout after %v waiting for loop_events confirmation", timeout)
		case frame := <-streamCh:
			ev, _ := frame.(map[string]interface{})
			if ev == nil {
				return nil, fmt.Errorf("connection closed waiting for loop_events confirmation")
			}
			typ, _ := ev["type"].(string)
			if typ == "error" {
				errObj, _ := ev["error"].(map[string]interface{})
				msg, _ := errObj["message"].(string)
				return nil, fmt.Errorf("daemon error: %s", msg)
			}
			payload, _ := ev["payload"].(map[string]interface{})
			return payload, nil
		default:
		}

		// No concurrent reader delivered yet; read synchronously. ReadEvent
		// routes other waiters' frames to the mux and returns the first frame
		// that belongs to nobody — which may be our own confirmation.
		ev, err := c.ReadEvent()
		if err != nil {
			return nil, fmt.Errorf("read event: %w", err)
		}
		if ev == nil {
			select {
			case frame := <-streamCh:
				ev, _ := frame.(map[string]interface{})
				if ev == nil {
					return nil, fmt.Errorf("connection closed waiting for loop_events confirmation")
				}
				typ, _ := ev["type"].(string)
				if typ == "error" {
					errObj, _ := ev["error"].(map[string]interface{})
					msg, _ := errObj["message"].(string)
					return nil, fmt.Errorf("daemon error: %s", msg)
				}
				payload, _ := ev["payload"].(map[string]interface{})
				return payload, nil
			default:
				return nil, fmt.Errorf("connection closed waiting for loop_events confirmation")
			}
		}
		typ, _ := ev["type"].(string)
		evID, _ := ev["id"].(string)
		if typ == "error" && evID == env.ID {
			errObj, _ := ev["error"].(map[string]interface{})
			msg, _ := errObj["message"].(string)
			return nil, fmt.Errorf("daemon error: %s", msg)
		}
		if typ != "next" || evID != env.ID {
			// Not ours — re-route defensively in case a waiter appeared.
			c.mux.route(ev)
			continue
		}
		payload, _ := ev["payload"].(map[string]interface{})
		return payload, nil
	}
}

// LoopDetach detaches from a loop (unsubscribe by subscription id) and waits
// for the daemon response.
//
// Multiplexer-aware: when a ReceiveMessages reader is active, the response is
// routed via the mux RPC channel instead of calling ReadEvent.
func (c *Client) LoopDetach(ctx context.Context, loopID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	env := NewUnsubscribeEnvelope(loopID)
	if err := c.SendMessage(ctx, env); err != nil {
		return nil, fmt.Errorf("send unsubscribe: %w", err)
	}

	// Register a pending call so a concurrent reader can deliver the response.
	pc, unregister := c.mux.registerRPC(env.ID)
	defer unregister()

	if c.conn != nil {
		_ = c.conn.SetReadDeadline(time.Now().Add(timeout))
		defer func() { _ = c.conn.SetReadDeadline(time.Time{}) }()
	}
	timeoutCh := time.After(timeout)
	for {
		// If a ReceiveMessages reader is active, it owns the socket read; wait
		// purely on the mux channels (gorilla/websocket forbids concurrent
		// readers, so LoopDetach must not call ReadEvent here).
		if c.readerActive.Load() {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-timeoutCh:
				return nil, fmt.Errorf("timeout after %v waiting for loop_detach", timeout)
			case result := <-pc.replyCh:
				return result, nil
			case err := <-pc.errCh:
				return nil, err
			}
		}

		// No concurrent reader: drain any already-routed response first.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeoutCh:
			return nil, fmt.Errorf("timeout after %v waiting for loop_detach", timeout)
		case result := <-pc.replyCh:
			return result, nil
		case err := <-pc.errCh:
			return nil, err
		default:
		}

		// No concurrent reader delivered yet; read synchronously.
		ev, err := c.ReadEvent()
		if err != nil {
			return nil, fmt.Errorf("read event: %w", err)
		}
		if ev == nil {
			select {
			case result := <-pc.replyCh:
				return result, nil
			case err := <-pc.errCh:
				return nil, err
			default:
				return nil, fmt.Errorf("connection closed waiting for loop_detach")
			}
		}
		evID, _ := ev["id"].(string)
		if evID != env.ID {
			// Not ours — re-route defensively.
			c.mux.route(ev)
			continue
		}
		typ, _ := ev["type"].(string)
		if typ == "error" {
			errObj, _ := ev["error"].(map[string]interface{})
			msg, _ := errObj["message"].(string)
			return nil, fmt.Errorf("daemon error: %s", msg)
		}
		if typ == "response" {
			result, _ := ev["result"].(map[string]interface{})
			if result == nil {
				result = ev
			}
			return result, nil
		}
	}
}

// LoopNew creates a new loop and waits for the response.
// Optional workspace and user can be set via the returned map.
func (c *Client) LoopNew(ctx context.Context, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type": "loop_new",
	}, "loop_new_response", timeout)
}

// LoopInput sends input to a loop and waits for the response.
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
// RPC Command convenience methods
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

// ---------------------------------------------------------------------------
// Additional loop methods
// ---------------------------------------------------------------------------

// LoopMessages requests persisted conversation/activity rows for a loop.
func (c *Client) LoopMessages(ctx context.Context, loopID string, limit int, offset int, includeEvents bool, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	payload := map[string]interface{}{
		"type":    "loop_messages",
		"loop_id": loopID,
	}
	if limit > 0 {
		payload["limit"] = limit
	}
	if offset > 0 {
		payload["offset"] = offset
	}
	if includeEvents {
		payload["include_events"] = true
	}
	return c.RequestResponse(ctx, payload, "loop_messages_response", timeout)
}

// LoopStateGet requests LangGraph checkpoint channel values.
func (c *Client) LoopStateGet(ctx context.Context, loopID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":    "loop_state_get",
		"loop_id": loopID,
	}, "loop_state_get_response", timeout)
}

// LoopStateUpdate applies partial checkpoint values to a loop.
func (c *Client) LoopStateUpdate(ctx context.Context, loopID string, values map[string]interface{}, asNode string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	payload := map[string]interface{}{
		"type":    "loop_state_update",
		"loop_id": loopID,
		"values":  values,
	}
	if asNode != "" {
		payload["as_node"] = asNode
	}
	return c.RequestResponse(ctx, payload, "loop_state_update_response", timeout)
}

// MCPStatus requests MCP server connection status.
func (c *Client) MCPStatus(ctx context.Context, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type": "mcp_status",
	}, "mcp_status_response", timeout)
}

// LoopHistoryFetch requests the loop's replayable history (goal display
// snapshots + live card tail) and waits for the response.
func (c *Client) LoopHistoryFetch(ctx context.Context, loopID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":    "loop_history_fetch",
		"loop_id": loopID,
	}, "loop_history_fetch_response", timeout)
}

// LoopExecutionStateFetch requests a focused execution-progress snapshot
// (plan, step_index, iteration, status) for the loop's bound checkpoint
// thread and waits for the response.
func (c *Client) LoopExecutionStateFetch(ctx context.Context, loopID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":    "loop_execution_state_fetch",
		"loop_id": loopID,
	}, "loop_execution_state_fetch_response", timeout)
}
