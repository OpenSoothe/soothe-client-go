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
