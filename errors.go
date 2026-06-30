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

// DisconnectCause distinguishes clean vs unclean connection loss (RFC-450 §4, §8.3).
// A clean drop follows a `disconnect` notification (loops keep running server-side);
// an unclean drop is a read/write error or a missed pong (in-flight queries are cancelled).
type DisconnectCause int

const (
	// DisconnectUnclean indicates an abrupt loss: read/write error or missed pong.
	DisconnectUnclean DisconnectCause = iota
	// DisconnectClean indicates a graceful peer-initiated disconnect notification.
	DisconnectClean
)

// String returns a human-readable cause name for logging.
func (d DisconnectCause) String() string {
	switch d {
	case DisconnectClean:
		return "clean"
	default:
		return "unclean"
	}
}

// ReconnectError indicates a failed reconnection attempt sequence.
type ReconnectError struct {
	URL      string
	Attempts int
	Err      error
}

func (e *ReconnectError) Error() string {
	return fmt.Sprintf("reconnect to %s failed after %d attempts: %v", e.URL, e.Attempts, e.Err)
}

func (e *ReconnectError) Unwrap() error {
	return e.Err
}

// NewReconnectError creates a new reconnect error.
func NewReconnectError(url string, attempts int, err error) *ReconnectError {
	return &ReconnectError{URL: url, Attempts: attempts, Err: err}
}

// StaleLoopError is returned by ReattachAndProbe when a loop accepts the
// reattach handshake but fails the loop_get liveness probe. Callers should
// fall back to a fresh loop_new bootstrap.
type StaleLoopError struct {
	LoopID string
	Err    error
}

func (e *StaleLoopError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("stale loop %s: reattach accepted but liveness probe failed: %v", e.LoopID, e.Err)
	}
	return fmt.Sprintf("stale loop %s: reattach accepted but liveness probe failed", e.LoopID)
}

func (e *StaleLoopError) Unwrap() error {
	return e.Err
}

// NewStaleLoopError creates a new stale-loop error.
func NewStaleLoopError(loopID string, err error) *StaleLoopError {
	return &StaleLoopError{LoopID: loopID, Err: err}
}
