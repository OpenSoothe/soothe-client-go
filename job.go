package soothe

import (
	"context"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Job IPC (RFC-228 Autopilot Job Management)
// ---------------------------------------------------------------------------

// JobCreate submits a root goal to AutopilotService, creating a new autopilot job.
// workspace is optional and resolves the filesystem root for goal execution.
func (c *Client) JobCreate(ctx context.Context, goal string, workspace string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if goal == "" {
		return nil, fmt.Errorf("goal is required")
	}
	payload := map[string]interface{}{
		"type": "job_create",
		"goal": goal,
	}
	if workspace != "" {
		payload["workspace"] = workspace
	}
	return c.RequestResponse(ctx, payload, "job_create_response", timeout)
}

// JobStatus queries job state: goal status, counts, assigned workers.
func (c *Client) JobStatus(ctx context.Context, jobID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if jobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":   "job_status",
		"job_id": jobID,
	}, "job_status_response", timeout)
}

// JobPause pauses goal execution by suspending the root goal.
func (c *Client) JobPause(ctx context.Context, jobID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if jobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":   "job_pause",
		"job_id": jobID,
	}, "job_pause_response", timeout)
}

// JobResume resumes paused goal execution by reactivating the root goal.
func (c *Client) JobResume(ctx context.Context, jobID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if jobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":   "job_resume",
		"job_id": jobID,
	}, "job_resume_response", timeout)
}

// JobCancel cancels job by cancelling the root goal via AutopilotService.
func (c *Client) JobCancel(ctx context.Context, jobID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if jobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":   "job_cancel",
		"job_id": jobID,
	}, "job_cancel_response", timeout)
}

// JobDag gets GoalEngine DAG snapshot for visualization.
func (c *Client) JobDag(ctx context.Context, jobID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if jobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":   "job_dag",
		"job_id": jobID,
	}, "job_dag_response", timeout)
}

// JobGuidance sends user guidance to GoalEngine for absorption.
// goalID is optional - if empty, targets the root job goal.
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
		"type":   "job_guidance",
		"job_id": jobID,
		"text":   text,
	}
	if goalID != "" {
		payload["goal_id"] = goalID
	}
	return c.RequestResponse(ctx, payload, "job_guidance_response", timeout)
}

// AutopilotSubscribe subscribes client to autopilot worker events.
// This bypasses the autopilot__* filter so the client receives worker lifecycle events.
func (c *Client) AutopilotSubscribe(ctx context.Context, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type": "autopilot_subscribe",
	}, "autopilot_subscribe_response", timeout)
}

// AutopilotUnsubscribe releases autopilot worker event subscription.
func (c *Client) AutopilotUnsubscribe(ctx context.Context, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type": "autopilot_unsubscribe",
	}, "autopilot_unsubscribe_response", timeout)
}
