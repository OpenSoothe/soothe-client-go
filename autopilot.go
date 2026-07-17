package soothe

import (
	"context"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Autopilot goal RPCs (protocol-1 request methods; mirrors Python CommandClient)
// ---------------------------------------------------------------------------

func autopilotTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return 15 * time.Second
	}
	return timeout
}

// AutopilotStatus returns autopilot scheduler status (running / dreaming / pool).
func (c *Client) AutopilotStatus(ctx context.Context, timeout time.Duration) (map[string]interface{}, error) {
	return c.RequestResponse(ctx, map[string]interface{}{
		"type": "autopilot_status",
	}, "autopilot_status", autopilotTimeout(timeout))
}

// AutopilotSubmit submits a new autopilot goal (returns goal_id).
func (c *Client) AutopilotSubmit(ctx context.Context, description string, priority int, workspace string, timeout time.Duration) (map[string]interface{}, error) {
	if description == "" {
		return nil, fmt.Errorf("description is required")
	}
	if priority <= 0 {
		priority = 50
	}
	payload := map[string]interface{}{
		"type":        "autopilot_submit",
		"description": description,
		"priority":    priority,
	}
	if workspace != "" {
		payload["workspace"] = workspace
	}
	return c.RequestResponse(ctx, payload, "autopilot_submit", autopilotTimeout(timeout))
}

// AutopilotListGoals lists all goals (including non-root children).
func (c *Client) AutopilotListGoals(ctx context.Context, timeout time.Duration) (map[string]interface{}, error) {
	return c.RequestResponse(ctx, map[string]interface{}{
		"type": "autopilot_list_goals",
	}, "autopilot_list_goals", autopilotTimeout(timeout))
}

// AutopilotGetGoal fetches one goal by id.
func (c *Client) AutopilotGetGoal(ctx context.Context, goalID string, timeout time.Duration) (map[string]interface{}, error) {
	if goalID == "" {
		return nil, fmt.Errorf("goal_id is required")
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":    "autopilot_get_goal",
		"goal_id": goalID,
	}, "autopilot_get_goal", autopilotTimeout(timeout))
}

// AutopilotCancelGoal cancels a goal and its non-terminal descendants.
func (c *Client) AutopilotCancelGoal(ctx context.Context, goalID string, timeout time.Duration) (map[string]interface{}, error) {
	if goalID == "" {
		return nil, fmt.Errorf("goal_id is required")
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":    "autopilot_cancel_goal",
		"goal_id": goalID,
	}, "autopilot_cancel_goal", autopilotTimeout(timeout))
}

// AutopilotCancelAll cancels every open (non-terminal) goal.
func (c *Client) AutopilotCancelAll(ctx context.Context, timeout time.Duration) (map[string]interface{}, error) {
	return c.RequestResponse(ctx, map[string]interface{}{
		"type": "autopilot_cancel_all",
	}, "autopilot_cancel_all", autopilotTimeout(timeout))
}

// AutopilotWake exits dreaming mode and resumes scheduling.
func (c *Client) AutopilotWake(ctx context.Context, timeout time.Duration) (map[string]interface{}, error) {
	return c.RequestResponse(ctx, map[string]interface{}{
		"type": "autopilot_wake",
	}, "autopilot_wake", autopilotTimeout(timeout))
}

// AutopilotDream forces dreaming mode.
func (c *Client) AutopilotDream(ctx context.Context, timeout time.Duration) (map[string]interface{}, error) {
	return c.RequestResponse(ctx, map[string]interface{}{
		"type": "autopilot_dream",
	}, "autopilot_dream", autopilotTimeout(timeout))
}

// AutopilotResume resumes a suspended or blocked goal.
func (c *Client) AutopilotResume(ctx context.Context, goalID string, timeout time.Duration) (map[string]interface{}, error) {
	if goalID == "" {
		return nil, fmt.Errorf("goal_id is required")
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":    "autopilot_resume",
		"goal_id": goalID,
	}, "autopilot_resume", autopilotTimeout(timeout))
}

// AutopilotListJobs lists root goals only (jobs). Prefer Job* for job control.
func (c *Client) AutopilotListJobs(ctx context.Context, timeout time.Duration) (map[string]interface{}, error) {
	return c.RequestResponse(ctx, map[string]interface{}{
		"type": "autopilot_list_jobs",
	}, "autopilot_list_jobs", autopilotTimeout(timeout))
}

// AutopilotGetJob gets a root job with DAG snapshot. Prefer JobStatus / JobDag.
func (c *Client) AutopilotGetJob(ctx context.Context, jobID string, timeout time.Duration) (map[string]interface{}, error) {
	if jobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}
	return c.RequestResponse(ctx, map[string]interface{}{
		"type":   "autopilot_get_job",
		"job_id": jobID,
	}, "autopilot_get_job", autopilotTimeout(timeout))
}
