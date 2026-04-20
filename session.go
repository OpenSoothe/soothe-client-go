package soothe

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mirasurf/lepton/soothe-client-go/config"
	"github.com/mirasurf/lepton/soothe-client-go/protocol"
)

// BootstrapNewThreadSession runs the daemon ready handshake → new_thread →
// subscribe_thread flow, returning the thread ID on success.
// This mirrors soothe_sdk.client.session.bootstrap_thread_session.
func BootstrapNewThreadSession(
	ctx context.Context,
	client *Client,
	eventCh <-chan interface{},
	workspace string,
	cfg *config.Config,
) (string, error) {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	// Step 1: daemon_ready handshake
	if err := client.SendMessage(ctx, protocol.BaseMessage{Type: "daemon_ready"}); err != nil {
		return "", fmt.Errorf("daemon_ready: %w", err)
	}
	if err := WaitDaemonReady(ctx, eventCh, cfg.DaemonReadyTimeout); err != nil {
		return "", err
	}

	// Step 2: new_thread
	if err := client.SendMessage(ctx, protocol.NewNewThreadMessage(workspace)); err != nil {
		return "", fmt.Errorf("new_thread: %w", err)
	}
	status, err := WaitThreadStatusWithID(ctx, eventCh, cfg.ThreadStatusTimeout)
	if err != nil {
		return "", err
	}
	tid := status.ThreadID
	if tid == "" {
		return "", fmt.Errorf("empty thread_id in status response")
	}

	// Step 3: subscribe_thread
	if err := client.SendMessage(ctx, protocol.NewSubscribeThreadMessage(tid, cfg.VerbosityLevel)); err != nil {
		return "", fmt.Errorf("subscribe_thread: %w", err)
	}
	if err := WaitSubscriptionConfirmed(ctx, eventCh, tid, cfg.VerbosityLevel, cfg.SubscriptionTimeout); err != nil {
		return "", err
	}

	return tid, nil
}

// BootstrapResumeThreadSession runs the daemon ready handshake → resume_thread →
// subscribe_thread flow, returning the thread ID on success.
func BootstrapResumeThreadSession(
	ctx context.Context,
	client *Client,
	eventCh <-chan interface{},
	threadID string,
	workspace string,
	cfg *config.Config,
) (string, error) {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	// Step 1: daemon_ready handshake
	if err := client.SendMessage(ctx, protocol.BaseMessage{Type: "daemon_ready"}); err != nil {
		return "", fmt.Errorf("daemon_ready: %w", err)
	}
	if err := WaitDaemonReady(ctx, eventCh, cfg.DaemonReadyTimeout); err != nil {
		return "", err
	}

	// Step 2: resume_thread
	if err := client.SendMessage(ctx, protocol.NewResumeThreadMessage(threadID, workspace)); err != nil {
		return "", fmt.Errorf("resume_thread: %w", err)
	}
	status, err := WaitThreadStatusWithID(ctx, eventCh, cfg.ThreadStatusTimeout)
	if err != nil {
		return "", err
	}
	tid := status.ThreadID
	if tid == "" {
		return "", fmt.Errorf("empty thread_id in status response")
	}

	// Step 3: subscribe_thread
	if err := client.SendMessage(ctx, protocol.NewSubscribeThreadMessage(tid, cfg.VerbosityLevel)); err != nil {
		return "", fmt.Errorf("subscribe_thread: %w", err)
	}
	if err := WaitSubscriptionConfirmed(ctx, eventCh, tid, cfg.VerbosityLevel, cfg.SubscriptionTimeout); err != nil {
		return "", err
	}

	return tid, nil
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
			case protocol.DaemonReadyResponse:
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

// WaitThreadStatusWithID waits for type status with non-empty thread_id.
func WaitThreadStatusWithID(ctx context.Context, ch <-chan interface{}, timeout time.Duration) (protocol.StatusResponse, error) {
	var zero protocol.StatusResponse
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
			return zero, fmt.Errorf("timeout after %v waiting for status with thread_id", timeout)
		case msg := <-ch:
			if msg == nil {
				continue
			}
			switch m := msg.(type) {
			case protocol.ErrorResponse:
				return zero, fmt.Errorf("daemon error: %s: %s", m.Code, m.Message)
			case protocol.StatusResponse:
				if m.ThreadID != "" {
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
					decoded, err := protocol.DecodeMessage(raw)
					if err != nil {
						continue
					}
					if st, ok := decoded.(protocol.StatusResponse); ok && st.ThreadID != "" {
						return st, nil
					}
				}
			}
		}
	}
}

// WaitSubscriptionConfirmed waits for subscription_confirmed matching thread_id.
func WaitSubscriptionConfirmed(ctx context.Context, ch <-chan interface{}, wantThreadID, wantVerbosity string, timeout time.Duration) error {
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
			return fmt.Errorf("timeout after %v waiting for subscription_confirmed", timeout)
		case msg := <-ch:
			if msg == nil {
				continue
			}
			switch m := msg.(type) {
			case protocol.SubscriptionConfirmedResponse:
				if m.ThreadID == wantThreadID {
					return nil
				}
			case map[string]interface{}:
				if t, _ := m["type"].(string); t == "subscription_confirmed" {
					tid, _ := m["thread_id"].(string)
					if tid == wantThreadID {
						return nil
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
