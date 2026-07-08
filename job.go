package soothe

import (
	"context"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Job IPC (RFC-228 Autopilot Job Management)
// ---------------------------------------------------------------------------

// SendJobCreate submits a root goal to AutopilotService, creating a new autopilot job.
// workspace is optional and resolves the filesystem root for goal execution.
func (c *Client) SendJobCreate(ctx context.Context, goal string, workspace string, requestID ...string) error {
	if goal == "" {
		return fmt.Errorf("goal is required")
	}
	rid := optRequestID(requestID)
	payload := map[string]interface{}{
		"goal": goal,
	}
	if workspace != "" {
		payload["workspace"] = workspace
	}
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("job_create", payload, rid))
}

// SendJobStatus queries job state: goal status, counts, assigned workers.
func (c *Client) SendJobStatus(ctx context.Context, jobID string, requestID ...string) error {
	if jobID == "" {
		return fmt.Errorf("job_id is required")
	}
	rid := optRequestID(requestID)
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("job_status", map[string]interface{}{
		"job_id": jobID,
	}, rid))
}

// SendJobPause pauses goal execution by suspending the root goal.
func (c *Client) SendJobPause(ctx context.Context, jobID string, requestID ...string) error {
	if jobID == "" {
		return fmt.Errorf("job_id is required")
	}
	rid := optRequestID(requestID)
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("job_pause", map[string]interface{}{
		"job_id": jobID,
	}, rid))
}

// SendJobResume resumes paused goal execution by reactivating the root goal.
func (c *Client) SendJobResume(ctx context.Context, jobID string, requestID ...string) error {
	if jobID == "" {
		return fmt.Errorf("job_id is required")
	}
	rid := optRequestID(requestID)
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("job_resume", map[string]interface{}{
		"job_id": jobID,
	}, rid))
}

// SendJobCancel cancels job by cancelling the root goal via AutopilotService.
func (c *Client) SendJobCancel(ctx context.Context, jobID string, requestID ...string) error {
	if jobID == "" {
		return fmt.Errorf("job_id is required")
	}
	rid := optRequestID(requestID)
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("job_cancel", map[string]interface{}{
		"job_id": jobID,
	}, rid))
}

// SendJobDag gets GoalEngine DAG snapshot for visualization.
func (c *Client) SendJobDag(ctx context.Context, jobID string, requestID ...string) error {
	if jobID == "" {
		return fmt.Errorf("job_id is required")
	}
	rid := optRequestID(requestID)
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("job_dag", map[string]interface{}{
		"job_id": jobID,
	}, rid))
}

// ---------------------------------------------------------------------------
// Convenience methods with blocking response
// ---------------------------------------------------------------------------

// JobCreate submits a root goal and waits for response.
func (c *Client) JobCreate(ctx context.Context, goal string, workspace string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if err := c.SendJobCreate(ctx, goal, workspace); err != nil {
		return nil, err
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type": "job_create",
		"goal": goal,
	}, "job_create_response", timeout)
}

// JobStatus queries job state and waits for response.
func (c *Client) JobStatus(ctx context.Context, jobID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if err := c.SendJobStatus(ctx, jobID); err != nil {
		return nil, err
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":   "job_status",
		"job_id": jobID,
	}, "job_status_response", timeout)
}

// JobPause pauses goal execution and waits for response.
func (c *Client) JobPause(ctx context.Context, jobID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if err := c.SendJobPause(ctx, jobID); err != nil {
		return nil, err
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":   "job_pause",
		"job_id": jobID,
	}, "job_pause_response", timeout)
}

// JobResume resumes paused goal execution and waits for response.
func (c *Client) JobResume(ctx context.Context, jobID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if err := c.SendJobResume(ctx, jobID); err != nil {
		return nil, err
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":   "job_resume",
		"job_id": jobID,
	}, "job_resume_response", timeout)
}

// JobCancel cancels job and waits for response.
func (c *Client) JobCancel(ctx context.Context, jobID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if err := c.SendJobCancel(ctx, jobID); err != nil {
		return nil, err
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":   "job_cancel",
		"job_id": jobID,
	}, "job_cancel_response", timeout)
}

// JobDag gets GoalEngine DAG snapshot and waits for response.
func (c *Client) JobDag(ctx context.Context, jobID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if err := c.SendJobDag(ctx, jobID); err != nil {
		return nil, err
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":   "job_dag",
		"job_id": jobID,
	}, "job_dag_response", timeout)
}

// JobGuidance sends user guidance to GoalEngine for absorption.
// goalID is optional - if empty, targets the root job goal.
// The canonical wire field for the guidance text is "content" (RFC-450 §10.1).
func (c *Client) JobGuidance(ctx context.Context, jobID string, text string, goalID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if jobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}
	if text == "" {
		return nil, fmt.Errorf("text is required")
	}
	payload := map[string]interface{}{
		"type":    "job_guidance",
		"job_id":  jobID,
		"content": text,
	}
	if goalID != "" {
		payload["goal_id"] = goalID
	}
	return c.RequestResponse(ctx, payload, "job_guidance_response", timeout)
}

// AutopilotSubscribe subscribes client to autopilot worker events.
// This bypasses the autopilot__* filter so the client receives worker lifecycle events.
// Uses the subscribe envelope with method "autopilot_events" (RFC-450 §9.2).
//
// Multiplexer-aware: when a ReceiveMessages reader is active, the confirmation
// frame is routed via the mux subscription channel instead of calling ReadEvent.
func (c *Client) AutopilotSubscribe(ctx context.Context, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	env := NewSubscribeEnvelope("autopilot_events", map[string]interface{}{})
	if err := c.SendMessage(ctx, env); err != nil {
		return "", fmt.Errorf("send subscribe: %w", err)
	}

	// Register a pending subscription so a concurrent reader can deliver the
	// confirmation next frame.
	streamCh, unsub := c.mux.registerSubscription(env.ID)
	defer unsub()

	if c.conn != nil {
		c.conn.SetReadDeadline(time.Now().Add(timeout))
		defer c.conn.SetReadDeadline(time.Time{})
	}
	timeoutCh := time.After(timeout)
	for {
		// If a ReceiveMessages reader is active, it owns the socket read; wait
		// purely on the mux channels (gorilla/websocket forbids concurrent
		// readers, so AutopilotSubscribe must not call ReadEvent here).
		if c.readerActive.Load() {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-timeoutCh:
				return "", fmt.Errorf("timeout after %v waiting for autopilot subscribe", timeout)
			case frame := <-streamCh:
				ev, _ := frame.(map[string]interface{})
				if ev == nil {
					return "", fmt.Errorf("connection closed waiting for autopilot subscribe")
				}
				evType, _ := ev["type"].(string)
				if evType == "error" {
					errObj, _ := ev["error"].(map[string]interface{})
					msg, _ := errObj["message"].(string)
					return "", fmt.Errorf("autopilot subscribe error: %s", msg)
				}
				// next or response with matching id means subscription accepted.
				return env.ID, nil
			}
		}

		// No concurrent reader: drain any already-routed frame first.
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timeoutCh:
			return "", fmt.Errorf("timeout after %v waiting for autopilot subscribe", timeout)
		case frame := <-streamCh:
			ev, _ := frame.(map[string]interface{})
			if ev == nil {
				return "", fmt.Errorf("connection closed waiting for autopilot subscribe")
			}
			evType, _ := ev["type"].(string)
			if evType == "error" {
				errObj, _ := ev["error"].(map[string]interface{})
				msg, _ := errObj["message"].(string)
				return "", fmt.Errorf("autopilot subscribe error: %s", msg)
			}
			return env.ID, nil
		default:
		}

		// No concurrent reader delivered yet; read synchronously.
		ev, err := c.ReadEvent()
		if err != nil {
			return "", fmt.Errorf("read event: %w", err)
		}
		if ev == nil {
			select {
			case frame := <-streamCh:
				ev, _ := frame.(map[string]interface{})
				if ev == nil {
					return "", fmt.Errorf("connection closed waiting for autopilot subscribe")
				}
				evType, _ := ev["type"].(string)
				if evType == "error" {
					errObj, _ := ev["error"].(map[string]interface{})
					msg, _ := errObj["message"].(string)
					return "", fmt.Errorf("autopilot subscribe error: %s", msg)
				}
				return env.ID, nil
			default:
				return "", fmt.Errorf("connection closed waiting for autopilot subscribe")
			}
		}
		evID, _ := ev["id"].(string)
		if evID != env.ID {
			// Not ours — re-route defensively.
			c.mux.route(ev)
			continue
		}
		evType, _ := ev["type"].(string)
		if evType == "error" {
			errObj, _ := ev["error"].(map[string]interface{})
			msg, _ := errObj["message"].(string)
			return "", fmt.Errorf("autopilot subscribe error: %s", msg)
		}
		// next or response with matching id means subscription accepted.
		return env.ID, nil
	}
}

// AutopilotUnsubscribe releases autopilot worker event subscription.
// Uses the unsubscribe envelope (RFC-450 §9.2): the daemon infers
// autopilot_unsubscribe from an unsubscribe with no loop_id in params.
func (c *Client) AutopilotUnsubscribe(ctx context.Context, subscriptionID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if subscriptionID == "" {
		return nil, fmt.Errorf("subscriptionID is required")
	}
	// Register a pending call so a concurrent reader can deliver the response.
	pc, unregister := c.mux.registerRPC(subscriptionID)
	defer unregister()

	if err := c.SendMessage(ctx, NewUnsubscribeEnvelope(subscriptionID)); err != nil {
		return nil, fmt.Errorf("send unsubscribe: %w", err)
	}

	// Set read deadline for response.
	if c.conn != nil {
		c.conn.SetReadDeadline(time.Now().Add(timeout))
		defer c.conn.SetReadDeadline(time.Time{})
	}

	timeoutCh := time.After(timeout)
	for {
		if c.readerActive.Load() {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-timeoutCh:
				return nil, fmt.Errorf("timeout after %v waiting for autopilot_unsubscribe", timeout)
			case result := <-pc.replyCh:
				return result, nil
			case err := <-pc.errCh:
				return nil, err
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeoutCh:
			return nil, fmt.Errorf("timeout after %v waiting for autopilot_unsubscribe", timeout)
		case result := <-pc.replyCh:
			return result, nil
		case err := <-pc.errCh:
			return nil, err
		default:
		}
		ev, err := c.ReadEvent()
		if err != nil {
			return nil, fmt.Errorf("read event: %w", err)
		}
		if ev == nil {
			select {
			case result := <-pc.replyCh:
				return result, nil
			default:
			}
			continue
		}
	}
}

// ---------------------------------------------------------------------------
// Cron IPC (RFC-229)
// ---------------------------------------------------------------------------

// CronAdd creates a scheduled job from natural language.
func (c *Client) CronAdd(ctx context.Context, text string, priority int, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if text == "" {
		return nil, fmt.Errorf("text is required")
	}
	payload := map[string]interface{}{
		"type": "cron_add",
		"text": text,
	}
	if priority > 0 {
		payload["priority"] = priority
	}
	return c.RequestResponse(ctx, payload, "cron_add_response", timeout)
}

// CronList lists scheduled jobs.
func (c *Client) CronList(ctx context.Context, status string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	payload := map[string]interface{}{
		"type": "cron_list",
	}
	if status != "" {
		payload["status"] = status
	}
	return c.RequestResponse(ctx, payload, "cron_list_response", timeout)
}

// CronShow shows a specific scheduled job.
func (c *Client) CronShow(ctx context.Context, jobID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if jobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":   "cron_show",
		"job_id": jobID,
	}, "cron_show_response", timeout)
}

// CronCancel cancels a scheduled job.
func (c *Client) CronCancel(ctx context.Context, jobID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if jobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":   "cron_cancel",
		"job_id": jobID,
	}, "cron_cancel_response", timeout)
}
