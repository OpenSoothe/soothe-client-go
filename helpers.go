package soothe

import (
	"context"
	"fmt"
	"time"
)

// CheckDaemonStatus checks daemon status via RPC (mirrors soothe_sdk.client.helpers.check_daemon_status).
func CheckDaemonStatus(ctx context.Context, client *Client, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return client.RequestResponse(ctx, map[string]interface{}{"type": "daemon_status"}, "daemon_status_response", timeout)
}

// IsDaemonLive performs a composite health check: connect + status RPC.
// Returns true if the daemon is live and responsive.
func IsDaemonLive(wsURL string, timeout time.Duration) bool {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	client := NewClient(wsURL, nil)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		return false
	}
	defer func() { _ = client.Close() }()

	_, err := CheckDaemonStatus(ctx, client, timeout)
	return err == nil
}

// RequestDaemonShutdown requests daemon shutdown via RPC
// (mirrors soothe_sdk.client.helpers.request_daemon_shutdown).
func RequestDaemonShutdown(ctx context.Context, client *Client, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	resp, err := client.RequestResponse(ctx, map[string]interface{}{"type": "daemon_shutdown"}, "shutdown_ack", timeout)
	if err != nil {
		return fmt.Errorf("shutdown failed: %w", err)
	}
	if status, _ := resp["status"].(string); status != "acknowledged" {
		return fmt.Errorf("shutdown not acknowledged: %v", resp)
	}
	return nil
}

// FetchSkillsCatalog fetches the skills catalog via RPC
// (mirrors soothe_sdk.client.helpers.fetch_skills_catalog).
func FetchSkillsCatalog(ctx context.Context, client *Client, timeout time.Duration) ([]map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	resp, err := client.RequestResponse(ctx, map[string]interface{}{"type": "skills_list"}, "skills_list_response", timeout)
	if err != nil {
		return nil, err
	}
	skillsRaw, ok := resp["skills"]
	if !ok {
		return nil, nil
	}
	skillsList, ok := skillsRaw.([]interface{})
	if !ok {
		return nil, nil
	}
	result := make([]map[string]interface{}, 0, len(skillsList))
	for _, s := range skillsList {
		if m, ok := s.(map[string]interface{}); ok {
			result = append(result, m)
		}
	}
	return result, nil
}

// FetchConfigSection fetches a daemon config section via RPC
// (mirrors soothe_sdk.client.helpers.fetch_config_section).
func FetchConfigSection(ctx context.Context, client *Client, section string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	resp, err := client.RequestResponse(ctx, map[string]interface{}{
		"type":    "config_get",
		"section": section,
	}, "config_get_response", timeout)
	if err != nil {
		return nil, err
	}
	if sec, ok := resp[section]; ok {
		if m, ok := sec.(map[string]interface{}); ok {
			return m, nil
		}
	}
	return resp, nil
}

// RequestDaemonConfigReload requests a config reload on the daemon via RPC
// (mirrors soothe_sdk.client.helpers.request_daemon_config_reload).
func RequestDaemonConfigReload(ctx context.Context, client *Client, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	resp, err := client.RequestResponse(ctx, map[string]interface{}{"type": "config_reload"}, "config_reload_response", timeout)
	if err != nil {
		return fmt.Errorf("config reload failed: %w", err)
	}
	if status, _ := resp["status"].(string); status != "reloaded" && status != "ok" {
		return fmt.Errorf("config reload not acknowledged: %v", resp)
	}
	return nil
}

// FetchLoopHistory requests a loop's replayable history via RPC.
func FetchLoopHistory(ctx context.Context, client *Client, loopID string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if loopID == "" {
		return nil, fmt.Errorf("loop_id is required")
	}
	return client.RequestResponse(ctx, map[string]interface{}{
		"type":    "loop_history_fetch",
		"loop_id": loopID,
	}, "loop_history_fetch_response", timeout)
}

// RequestAuth submits access_key/secret_key credentials to the daemon via RPC.
func RequestAuth(ctx context.Context, client *Client, accessKey, secretKey string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return client.RequestResponse(ctx, map[string]interface{}{
		"type":       "auth",
		"access_key": accessKey,
		"secret_key": secretKey,
	}, "auth_response", timeout)
}

// RequestAuthRefresh submits a refresh_token to the daemon via RPC.
func RequestAuthRefresh(ctx context.Context, client *Client, refreshToken string, timeout time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return client.RequestResponse(ctx, map[string]interface{}{
		"type":          "auth_refresh",
		"refresh_token": refreshToken,
	}, "auth_refresh_response", timeout)
}
