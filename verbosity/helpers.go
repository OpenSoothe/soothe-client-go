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
	defer client.Close()

	_, err := CheckDaemonStatus(ctx, client, timeout)
	if err != nil {
		return false
	}
	return true
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
