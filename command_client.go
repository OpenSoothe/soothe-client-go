package soothe

import (
	"context"
	"fmt"
	"time"
)

// CommandClient performs ephemeral one-shot RPCs (jobs / cron / autopilot)
// without holding a streaming subscription. Mirrors Python CommandClient.
type CommandClient struct {
	URL     string
	Timeout time.Duration
}

// NewCommandClient creates an ephemeral RPC client for the daemon URL.
func NewCommandClient(url string, timeout time.Duration) *CommandClient {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &CommandClient{URL: url, Timeout: timeout}
}

func (cc *CommandClient) withClient(ctx context.Context, fn func(context.Context, *Client) (map[string]interface{}, error)) (map[string]interface{}, error) {
	client := NewClient(cc.URL, nil)
	defer func() { _ = client.Close() }()
	if err := ConnectWithRetries(ctx, client, 5, 250*time.Millisecond); err != nil {
		return nil, err
	}
	rpcCtx, cancel := context.WithTimeout(ctx, cc.Timeout)
	defer cancel()
	return fn(rpcCtx, client)
}

// JobCreate submits a root goal via a fresh connection.
func (cc *CommandClient) JobCreate(ctx context.Context, goal, workspace string) (map[string]interface{}, error) {
	return cc.withClient(ctx, func(ctx context.Context, c *Client) (map[string]interface{}, error) {
		return c.JobCreate(ctx, goal, workspace, cc.Timeout)
	})
}

// JobStatus queries job state via a fresh connection.
func (cc *CommandClient) JobStatus(ctx context.Context, jobID string) (map[string]interface{}, error) {
	return cc.withClient(ctx, func(ctx context.Context, c *Client) (map[string]interface{}, error) {
		return c.JobStatus(ctx, jobID, cc.Timeout)
	})
}

// JobCancel cancels a job via a fresh connection.
func (cc *CommandClient) JobCancel(ctx context.Context, jobID string) (map[string]interface{}, error) {
	return cc.withClient(ctx, func(ctx context.Context, c *Client) (map[string]interface{}, error) {
		return c.JobCancel(ctx, jobID, cc.Timeout)
	})
}

// AutopilotStatus returns autopilot scheduler status via a fresh connection.
func (cc *CommandClient) AutopilotStatus(ctx context.Context) (map[string]interface{}, error) {
	return cc.withClient(ctx, func(ctx context.Context, c *Client) (map[string]interface{}, error) {
		return c.AutopilotStatus(ctx, cc.Timeout)
	})
}

// AutopilotSubmit submits a new autopilot goal via a fresh connection.
func (cc *CommandClient) AutopilotSubmit(ctx context.Context, description string, priority int, workspace string) (map[string]interface{}, error) {
	return cc.withClient(ctx, func(ctx context.Context, c *Client) (map[string]interface{}, error) {
		return c.AutopilotSubmit(ctx, description, priority, workspace, cc.Timeout)
	})
}

// AutopilotListGoals lists all goals via a fresh connection.
func (cc *CommandClient) AutopilotListGoals(ctx context.Context) (map[string]interface{}, error) {
	return cc.withClient(ctx, func(ctx context.Context, c *Client) (map[string]interface{}, error) {
		return c.AutopilotListGoals(ctx, cc.Timeout)
	})
}

// AutopilotGetGoal fetches one goal by id via a fresh connection.
func (cc *CommandClient) AutopilotGetGoal(ctx context.Context, goalID string) (map[string]interface{}, error) {
	return cc.withClient(ctx, func(ctx context.Context, c *Client) (map[string]interface{}, error) {
		return c.AutopilotGetGoal(ctx, goalID, cc.Timeout)
	})
}

// AutopilotCancelGoal cancels a goal via a fresh connection.
func (cc *CommandClient) AutopilotCancelGoal(ctx context.Context, goalID string) (map[string]interface{}, error) {
	return cc.withClient(ctx, func(ctx context.Context, c *Client) (map[string]interface{}, error) {
		return c.AutopilotCancelGoal(ctx, goalID, cc.Timeout)
	})
}

// AutopilotCancelAll cancels every open goal via a fresh connection.
func (cc *CommandClient) AutopilotCancelAll(ctx context.Context) (map[string]interface{}, error) {
	return cc.withClient(ctx, func(ctx context.Context, c *Client) (map[string]interface{}, error) {
		return c.AutopilotCancelAll(ctx, cc.Timeout)
	})
}

// AutopilotWake exits dreaming mode via a fresh connection.
func (cc *CommandClient) AutopilotWake(ctx context.Context) (map[string]interface{}, error) {
	return cc.withClient(ctx, func(ctx context.Context, c *Client) (map[string]interface{}, error) {
		return c.AutopilotWake(ctx, cc.Timeout)
	})
}

// AutopilotDream forces dreaming mode via a fresh connection.
func (cc *CommandClient) AutopilotDream(ctx context.Context) (map[string]interface{}, error) {
	return cc.withClient(ctx, func(ctx context.Context, c *Client) (map[string]interface{}, error) {
		return c.AutopilotDream(ctx, cc.Timeout)
	})
}

// AutopilotResume resumes a suspended or blocked goal via a fresh connection.
func (cc *CommandClient) AutopilotResume(ctx context.Context, goalID string) (map[string]interface{}, error) {
	return cc.withClient(ctx, func(ctx context.Context, c *Client) (map[string]interface{}, error) {
		return c.AutopilotResume(ctx, goalID, cc.Timeout)
	})
}

// AutopilotListJobs lists root goals via a fresh connection.
func (cc *CommandClient) AutopilotListJobs(ctx context.Context) (map[string]interface{}, error) {
	return cc.withClient(ctx, func(ctx context.Context, c *Client) (map[string]interface{}, error) {
		return c.AutopilotListJobs(ctx, cc.Timeout)
	})
}

// AutopilotGetJob gets a root job with DAG snapshot via a fresh connection.
func (cc *CommandClient) AutopilotGetJob(ctx context.Context, jobID string) (map[string]interface{}, error) {
	return cc.withClient(ctx, func(ctx context.Context, c *Client) (map[string]interface{}, error) {
		return c.AutopilotGetJob(ctx, jobID, cc.Timeout)
	})
}

// CronAdd creates a scheduled job via a fresh connection.
func (cc *CommandClient) CronAdd(ctx context.Context, text string, priority int) (map[string]interface{}, error) {
	return cc.withClient(ctx, func(ctx context.Context, c *Client) (map[string]interface{}, error) {
		return c.CronAdd(ctx, text, priority, cc.Timeout)
	})
}

// CronList lists scheduled jobs via a fresh connection.
func (cc *CommandClient) CronList(ctx context.Context, status string) (map[string]interface{}, error) {
	return cc.withClient(ctx, func(ctx context.Context, c *Client) (map[string]interface{}, error) {
		return c.CronList(ctx, status, cc.Timeout)
	})
}

// Request is a generic one-shot RPC (method = payload["type"]).
func (cc *CommandClient) Request(ctx context.Context, method string, params map[string]interface{}) (map[string]interface{}, error) {
	if method == "" {
		return nil, fmt.Errorf("method is required")
	}
	payload := map[string]interface{}{"type": method}
	for k, v := range params {
		payload[k] = v
	}
	return cc.withClient(ctx, func(ctx context.Context, c *Client) (map[string]interface{}, error) {
		return c.RequestResponse(ctx, payload, method+"_response", cc.Timeout)
	})
}
