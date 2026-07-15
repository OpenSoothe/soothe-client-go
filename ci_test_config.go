package soothe

import (
	"os"
	"strconv"
	"time"
)

// CITestConfig provides CI-optimized test timeouts and durations.
// When SOOTHE_CI_MODE is set to "true" (default in CI environments),
// tests use shorter timeouts for faster execution.
type CITestConfig struct {
	// CI mode is enabled
	CI bool
	// Heartbeat threshold for testing (shorter in CI)
	HeartbeatThreshold time.Duration
	// Sleep duration for threshold tests
	ThresholdSleep time.Duration
	// Default timeout for test operations
	DefaultTimeout time.Duration
	// Retry delay for connection tests
	RetryDelay time.Duration
}

// ciConfig is the global CI test configuration
var ciConfig *CITestConfig

// GetCIConfig returns the CI test configuration, initializing it once.
func GetCIConfig() *CITestConfig {
	if ciConfig != nil {
		return ciConfig
	}

	ciMode := getEnvBool("SOOTHE_CI_MODE", true) // Default to CI mode for safety
	ciConfig = &CITestConfig{
		CI: ciMode,
	}

	if ciMode {
		// CI-optimized values (fast but still valid for testing)
		ciConfig.HeartbeatThreshold = 100 * time.Millisecond
		ciConfig.ThresholdSleep = 150 * time.Millisecond
		ciConfig.DefaultTimeout = 1 * time.Second
		ciConfig.RetryDelay = 10 * time.Millisecond
	} else {
		// Production-like values for thorough local testing
		ciConfig.HeartbeatThreshold = 10 * time.Second
		ciConfig.ThresholdSleep = 11 * time.Second
		ciConfig.DefaultTimeout = 5 * time.Second
		ciConfig.RetryDelay = 50 * time.Millisecond
	}

	return ciConfig
}

// getEnvBool reads a boolean environment variable with a default value.
func getEnvBool(key string, defaultValue bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultValue
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return defaultValue
	}
	return b
}
