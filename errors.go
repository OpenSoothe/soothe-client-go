package soothe

import "fmt"

// ConnectionError represents a WebSocket connection failure.
type ConnectionError struct {
	URL     string
	Attempt int
	Err     error
}

func (e *ConnectionError) Error() string {
	return fmt.Sprintf("connection error to %s (attempt %d): %v", e.URL, e.Attempt, e.Err)
}

func (e *ConnectionError) Unwrap() error {
	return e.Err
}

// NewConnectionError creates a new connection error.
func NewConnectionError(url string, attempt int, err error) *ConnectionError {
	return &ConnectionError{URL: url, Attempt: attempt, Err: err}
}

// DaemonError represents an error reported by the Soothe daemon (RFC-450 §7).
// The daemon's structured error object carries a numeric code from the
// reserved ranges, a human-readable message, and optional data.
type DaemonError struct {
	Code    int
	Message string
	Data    map[string]interface{}
}

func (e *DaemonError) Error() string {
	if len(e.Data) > 0 {
		return fmt.Sprintf("daemon error [%d]: %s (%v)", e.Code, e.Message, e.Data)
	}
	return fmt.Sprintf("daemon error [%d]: %s", e.Code, e.Message)
}

// NewDaemonError creates a new daemon error with a numeric code.
func NewDaemonError(code int, message string) *DaemonError {
	return &DaemonError{Code: code, Message: message}
}

// TimeoutError represents a timeout waiting for a daemon response.
type TimeoutError struct {
	Operation string
	Duration  string
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("timeout after %s waiting for %s", e.Duration, e.Operation)
}

// NewTimeoutError creates a new timeout error.
func NewTimeoutError(operation, duration string) *TimeoutError {
	return &TimeoutError{Operation: operation, Duration: duration}
}
