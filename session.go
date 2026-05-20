package soothe

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// BootstrapLoopSession runs daemon_ready → (loop_new or reuse resumeLoopID) → loop_subscribe.
// It mirrors soothe_sdk.client.session.bootstrap_loop_session and returns the loop id.
// Call this before starting a concurrent ReceiveMessages reader on the same connection.
func BootstrapLoopSession(
	ctx context.Context,
	client *Client,
	resumeLoopID string,
	workspace string,
	cfg *Config,
) (string, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	if err := client.SendMessage(ctx, BaseMessage{Type: "daemon_ready"}); err != nil {
		return "", fmt.Errorf("daemon_ready: %w", err)
	}
	if _, err := client.WaitForDaemonReady(cfg.DaemonReadyTimeout); err != nil {
		return "", err
	}

	loopID := strings.TrimSpace(resumeLoopID)
	if loopID == "" {
		newPayload := map[string]interface{}{"type": "loop_new"}
		if workspace != "" {
			newPayload["workspace"] = workspace
		}
		resp, err := client.RequestResponse(
			ctx,
			newPayload,
			"loop_new_response",
			cfg.LoopStatusTimeout,
		)
		if err != nil {
			return "", fmt.Errorf("loop_new: %w", err)
		}
		lid, _ := resp["loop_id"].(string)
		loopID = strings.TrimSpace(lid)
		if loopID == "" {
			return "", fmt.Errorf("loop_new_response missing loop_id")
		}
	}

	subPayload := map[string]interface{}{
		"type":      "loop_subscribe",
		"loop_id":   loopID,
		"verbosity": cfg.VerbosityLevel,
	}
	subResp, err := client.RequestResponse(ctx, subPayload, "loop_subscribe_response", cfg.SubscriptionTimeout)
	if err != nil {
		return "", fmt.Errorf("loop_subscribe: %w", err)
	}
	if ok, has := subResp["success"].(bool); has && !ok {
		return "", fmt.Errorf("loop_subscribe failed: %v", subResp)
	}

	return loopID, nil
}

// ---------------------------------------------------------------------------
// Wait helpers (consume from event channel)
// ---------------------------------------------------------------------------

// WaitDaemonReady blocks until a daemon_ready message with state == "ready".
func WaitDaemonReady(ctx context.Context, ch <-chan interface{}, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("timeout after %v waiting for daemon_ready (state=ready)", timeout)
		case msg := <-ch:
			if msg == nil {
				continue
			}
			switch m := msg.(type) {
			case DaemonReadyResponse:
				if m.State == "ready" {
					return nil
				}
				return fmt.Errorf("daemon not ready: state=%q message=%q", m.State, m.Message)
			case map[string]interface{}:
				if t, _ := m["type"].(string); t == "daemon_ready" {
					if st, _ := m["state"].(string); st == "ready" {
						return nil
					}
					return fmt.Errorf("daemon not ready: %#v", m)
				}
			}
		}
	}
}

// WaitLoopStatusWithID waits for type status with non-empty loop_id.
func WaitLoopStatusWithID(ctx context.Context, ch <-chan interface{}, timeout time.Duration) (StatusResponse, error) {
	var zero StatusResponse
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-deadline.C:
			return zero, fmt.Errorf("timeout after %v waiting for status with loop_id", timeout)
		case msg := <-ch:
			if msg == nil {
				continue
			}
			switch m := msg.(type) {
			case ErrorResponse:
				return zero, fmt.Errorf("daemon error: %s: %s", m.Code, m.Message)
			case StatusResponse:
				if m.LoopID != "" {
					return m, nil
				}
			case map[string]interface{}:
				typ, _ := m["type"].(string)
				if typ == "error" {
					code, _ := m["code"].(string)
					msgStr, _ := m["message"].(string)
					return zero, fmt.Errorf("daemon error: %s: %s", code, msgStr)
				}
				if typ == "status" {
					raw, err := json.Marshal(m)
					if err != nil {
						continue
					}
					decoded, err := DecodeMessage(raw)
					if err != nil {
						continue
					}
					if st, ok := decoded.(StatusResponse); ok && st.LoopID != "" {
						return st, nil
					}
				}
			}
		}
	}
}

// WaitSubscriptionConfirmed waits for subscription_confirmed or loop_subscribe_response matching loopID.
func WaitSubscriptionConfirmed(ctx context.Context, ch <-chan interface{}, wantLoopID, wantVerbosity string, timeout time.Duration) error {
	_ = wantVerbosity
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("timeout after %v waiting for subscription confirmation", timeout)
		case msg := <-ch:
			if msg == nil {
				continue
			}
			switch m := msg.(type) {
			case SubscriptionConfirmedResponse:
				if m.LoopID == wantLoopID {
					return nil
				}
			case map[string]interface{}:
				t, _ := m["type"].(string)
				if t == "subscription_confirmed" {
					lid, _ := m["loop_id"].(string)
					if lid == wantLoopID {
						return nil
					}
				}
				if t == "loop_subscribe_response" {
					if ok, _ := m["success"].(bool); ok {
						lid, _ := m["loop_id"].(string)
						if lid == wantLoopID {
							return nil
						}
					}
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Connect with retries (mirrors soothe_sdk.client.session.connect_websocket_with_retries)
// ---------------------------------------------------------------------------

// ConnectWithRetries attempts to connect to the Soothe daemon with bounded retries.
// This handles cold-start races where the daemon may not be ready yet.
func ConnectWithRetries(ctx context.Context, client *Client, maxRetries int, retryDelay time.Duration) error {
	if maxRetries <= 0 {
		maxRetries = 40
	}
	if retryDelay <= 0 {
		retryDelay = 250 * time.Millisecond
	}
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := client.Connect(connectCtx)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryDelay):
		}
	}
	return fmt.Errorf("failed to connect after %d attempts: %w", maxRetries, lastErr)
}