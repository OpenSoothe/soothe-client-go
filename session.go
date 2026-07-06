package soothe

import (
	"context"
	"fmt"
	"strings"
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

// asString coerces an interface{} to string when possible.
func asString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
