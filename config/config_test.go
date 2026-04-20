package config

import (
	"os"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.DaemonURL != "ws://localhost:8765" {
		t.Errorf("DaemonURL: %s", cfg.DaemonURL)
	}
	if cfg.VerbosityLevel != "normal" {
		t.Errorf("VerbosityLevel: %s", cfg.VerbosityLevel)
	}
	if cfg.MaxRetries != 5 {
		t.Errorf("MaxRetries: %d", cfg.MaxRetries)
	}
	if cfg.DaemonReadyTimeout != 20*time.Second {
		t.Errorf("DaemonReadyTimeout: %v", cfg.DaemonReadyTimeout)
	}
	if cfg.ThreadStatusTimeout != 60*time.Second {
		t.Errorf("ThreadStatusTimeout: %v", cfg.ThreadStatusTimeout)
	}
	if cfg.SubscriptionTimeout != 10*time.Second {
		t.Errorf("SubscriptionTimeout: %v", cfg.SubscriptionTimeout)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	// Set env vars
	os.Setenv("SOOTHE_DAEMON_URL", "ws://custom:9999")
	os.Setenv("SOOTHE_VERBOSITY", "debug")
	os.Setenv("SOOTHE_MAX_RETRIES", "10")
	os.Setenv("SOOTHE_DAEMON_READY_TIMEOUT_SEC", "30")
	os.Setenv("SOOTHE_THREAD_STATUS_TIMEOUT_SEC", "45")
	os.Setenv("SOOTHE_SUBSCRIPTION_TIMEOUT_SEC", "15")
	defer func() {
		os.Unsetenv("SOOTHE_DAEMON_URL")
		os.Unsetenv("SOOTHE_VERBOSITY")
		os.Unsetenv("SOOTHE_MAX_RETRIES")
		os.Unsetenv("SOOTHE_DAEMON_READY_TIMEOUT_SEC")
		os.Unsetenv("SOOTHE_THREAD_STATUS_TIMEOUT_SEC")
		os.Unsetenv("SOOTHE_SUBSCRIPTION_TIMEOUT_SEC")
	}()

	cfg := LoadConfigFromEnv()
	if cfg.DaemonURL != "ws://custom:9999" {
		t.Errorf("DaemonURL: %s", cfg.DaemonURL)
	}
	if cfg.VerbosityLevel != "debug" {
		t.Errorf("VerbosityLevel: %s", cfg.VerbosityLevel)
	}
	if cfg.MaxRetries != 10 {
		t.Errorf("MaxRetries: %d", cfg.MaxRetries)
	}
	if cfg.DaemonReadyTimeout != 30*time.Second {
		t.Errorf("DaemonReadyTimeout: %v", cfg.DaemonReadyTimeout)
	}
	if cfg.ThreadStatusTimeout != 45*time.Second {
		t.Errorf("ThreadStatusTimeout: %v", cfg.ThreadStatusTimeout)
	}
	if cfg.SubscriptionTimeout != 15*time.Second {
		t.Errorf("SubscriptionTimeout: %v", cfg.SubscriptionTimeout)
	}
}

func TestLoadConfigFromEnv_InvalidValues(t *testing.T) {
	os.Setenv("SOOTHE_MAX_RETRIES", "not-a-number")
	os.Setenv("SOOTHE_DAEMON_READY_TIMEOUT_SEC", "-5")
	defer func() {
		os.Unsetenv("SOOTHE_MAX_RETRIES")
		os.Unsetenv("SOOTHE_DAEMON_READY_TIMEOUT_SEC")
	}()

	cfg := LoadConfigFromEnv()
	// Should fall back to defaults
	if cfg.MaxRetries != 5 {
		t.Errorf("MaxRetries should default: %d", cfg.MaxRetries)
	}
	if cfg.DaemonReadyTimeout != 20*time.Second {
		t.Errorf("DaemonReadyTimeout should default: %v", cfg.DaemonReadyTimeout)
	}
}
