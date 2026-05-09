package soothe

import (
	"os"
	"strconv"
	"time"
)

// Config holds client configuration for connecting to the Soothe daemon.
type Config struct {
	DaemonURL          string        // WebSocket URL for Soothe daemon
	VerbosityLevel     string        // Event verbosity: quiet/normal/debug
	MaxRetries         int           // Maximum connection retry attempts
	ReconnectDelay     time.Duration // Initial reconnect delay
	HeartbeatInterval  time.Duration // Application-level heartbeat interval
	DaemonReadyTimeout time.Duration // Handshake: wait for daemon_ready
	LoopStatusTimeout  time.Duration // After loop_new: wait for status with loop_id
	SubscriptionTimeout time.Duration // After loop_subscribe: wait for subscription_confirmed
}

// DefaultConfig returns default configuration.
func DefaultConfig() *Config {
	return &Config{
		DaemonURL:          "ws://localhost:8765",
		VerbosityLevel:     "normal",
		MaxRetries:         5,
		ReconnectDelay:     2 * time.Second,
		HeartbeatInterval:  30 * time.Second,
		DaemonReadyTimeout: 20 * time.Second,
		LoopStatusTimeout:  60 * time.Second,
		SubscriptionTimeout: 10 * time.Second,
	}
}

// LoadConfigFromEnv loads configuration from environment variables.
func LoadConfigFromEnv() *Config {
	config := DefaultConfig()

	if url := os.Getenv("SOOTHE_DAEMON_URL"); url != "" {
		config.DaemonURL = url
	}
	if verbosity := os.Getenv("SOOTHE_VERBOSITY"); verbosity != "" {
		config.VerbosityLevel = verbosity
	}
	if retries := os.Getenv("SOOTHE_MAX_RETRIES"); retries != "" {
		if val, err := strconv.Atoi(retries); err == nil {
			config.MaxRetries = val
		}
	}
	if s := os.Getenv("SOOTHE_DAEMON_READY_TIMEOUT_SEC"); s != "" {
		if val, err := strconv.Atoi(s); err == nil && val > 0 {
			config.DaemonReadyTimeout = time.Duration(val) * time.Second
		}
	}
	if s := os.Getenv("SOOTHE_LOOP_STATUS_TIMEOUT_SEC"); s != "" {
		if val, err := strconv.Atoi(s); err == nil && val > 0 {
			config.LoopStatusTimeout = time.Duration(val) * time.Second
		}
	}
	if s := os.Getenv("SOOTHE_SUBSCRIPTION_TIMEOUT_SEC"); s != "" {
		if val, err := strconv.Atoi(s); err == nil && val > 0 {
			config.SubscriptionTimeout = time.Duration(val) * time.Second
		}
	}

	return config
}
