package soothe

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// LoopSessionOptions configures loop_new workspace fields (RFC-503).
type LoopSessionOptions struct {
	ClientWorkspace   string
	UserID            string
	ClientWorkspaceID string
}

// BootstrapLoopSession runs (loop_new or reuse resumeLoopID) → loop_subscribe.
// The protocol-1 connection_init/connection_ack handshake is performed in
// Connect(); this function assumes it has already completed. It mirrors
// soothe_sdk.client.session.bootstrap_loop_session and returns the loop id.
// Call this before starting a concurrent ReceiveMessages reader on the same connection.
func BootstrapLoopSession(
	ctx context.Context,
	client *Client,
	resumeLoopID string,
	clientWorkspace string,
	cfg *Config,
	opts ...*LoopSessionOptions,
) (string, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	var sessionOpts *LoopSessionOptions
	if len(opts) > 0 {
		sessionOpts = opts[0]
	}

	loopID := strings.TrimSpace(resumeLoopID)
	if loopID == "" {
		newPayload := map[string]interface{}{"type": "loop_new"}
		cw := strings.TrimSpace(clientWorkspace)
		if sessionOpts != nil && strings.TrimSpace(sessionOpts.ClientWorkspace) != "" {
			cw = strings.TrimSpace(sessionOpts.ClientWorkspace)
		}
		if cw != "" {
			newPayload["client_workspace"] = cw
		}
		if sessionOpts != nil {
			if uid := strings.TrimSpace(sessionOpts.UserID); uid != "" {
				newPayload["user_id"] = uid
			}
			if wsid := strings.TrimSpace(sessionOpts.ClientWorkspaceID); wsid != "" {
				newPayload["client_workspace_id"] = wsid
			}
		}
		resp, err := client.RequestResponse(
			ctx,
			newPayload,
			"loop_new",
			cfg.LoopStatusTimeout,
		)
		if err != nil {
			return "", fmt.Errorf("loop_new: %w", err)
		}
		lid, _ := resp["loop_id"].(string)
		loopID = strings.TrimSpace(lid)
		if loopID == "" {
			return "", fmt.Errorf("loop_new response missing loop_id")
		}
	}

	// Subscribe to the loop event stream. Confirmation arrives as a `next` frame.
	subResp, err := client.LoopSubscribe(ctx, loopID, cfg.VerbosityLevel, cfg.SubscriptionTimeout)
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

// WaitDaemonReady blocks until a connection_ack with readiness_state == "ready".
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
			return fmt.Errorf("timeout after %v waiting for connection_ack (ready)", timeout)
		case msg := <-ch:
			if msg == nil {
				continue
			}
			if m, ok := msg.(Envelope); ok {
				if m.Type == "connection_ack" {
					result := m.Result
					state, _ := result["readiness_state"].(string)
					if state == "ready" {
						return nil
					}
					return fmt.Errorf("daemon not ready: state=%q", state)
				}
				continue
			}
			if m, ok := msg.(map[string]interface{}); ok {
				if t, _ := m["type"].(string); t == "connection_ack" {
					result, _ := m["result"].(map[string]interface{})
					state, _ := result["readiness_state"].(string)
					if state == "ready" {
						return nil
					}
					return fmt.Errorf("daemon not ready: state=%q", state)
				}
			}
		}
	}
}

// WaitLoopStatusWithID waits for a status frame with non-empty loop_id.
// Under protocol-1 the daemon emits status frames as top-level type "status"
// (not wrapped in next).
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
			// Normalize to a map for uniform inspection.
			var m map[string]interface{}
			switch v := msg.(type) {
			case Envelope:
				if v.Type == "error" {
					msgStr, _ := v.Error.Message, v.Error
					return zero, fmt.Errorf("daemon error: %s", msgStr)
				}
				if v.Type == "status" {
					m = map[string]interface{}{"type": "status", "state": v.Params["state"], "loop_id": v.Params["loop_id"]}
				}
			case map[string]interface{}:
				m = v
			}
			if m == nil {
				continue
			}
			typ, _ := m["type"].(string)
			if typ == "error" {
				if errObj, ok := m["error"].(map[string]interface{}); ok {
					msgStr, _ := errObj["message"].(string)
					return zero, fmt.Errorf("daemon error: %s", msgStr)
				}
				msgStr, _ := m["message"].(string)
				return zero, fmt.Errorf("daemon error: %s", msgStr)
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
				// Fall back to map inspection.
				if lid, _ := m["loop_id"].(string); lid != "" {
					return StatusResponse{
						BaseMessage: BaseMessage{Type: "status"},
						State:       asString(m["state"]),
						LoopID:      lid,
						Workspace:   asString(m["workspace"]),
					}, nil
				}
			}
		}
	}
}

// WaitSubscriptionConfirmed waits for a subscription confirmation `next` frame
// whose payload carries the matching loop_id (RFC-450 §9.4).
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
			var payload map[string]interface{}
			var typ string
			switch v := msg.(type) {
			case Envelope:
				typ = v.Type
				if v.Type == "next" {
					payload = v.Payload
				}
				if v.Type == "error" {
					return fmt.Errorf("daemon error: %s", v.Error.Message)
				}
			case map[string]interface{}:
				typ, _ = v["type"].(string)
				if typ == "next" {
					payload, _ = v["payload"].(map[string]interface{})
				}
				if typ == "error" {
					if errObj, ok := v["error"].(map[string]interface{}); ok {
						return fmt.Errorf("daemon error: %s", errObj["message"])
					}
				}
			}
			if typ != "next" || payload == nil {
				continue
			}
			lid, _ := payload["loop_id"].(string)
			if lid != wantLoopID {
				continue
			}
			if ok, _ := payload["success"].(bool); ok {
				return nil
			}
		}
	}
}

// asString coerces an interface{} to string when possible.
func asString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
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
