package soothe_test

import (
	"errors"
	"fmt"
	"time"

	soothe "github.com/mirasoth/soothe-client-go"
)

// Example_connectionError constructs and inspects a ConnectionError, which
// wraps a dial failure with the target URL and attempt number.
func Example_connectionError() {
	cause := fmt.Errorf("dial tcp: connection refused")
	ce := soothe.NewConnectionError("ws://localhost:8765", 3, cause)
	fmt.Println(ce)
	fmt.Println("URL:", ce.URL)
	fmt.Println("Attempt:", ce.Attempt)
	fmt.Println("Unwrap:", errors.Unwrap(ce))
	// Output:
	// connection error to ws://localhost:8765 (attempt 3): dial tcp: connection refused
	// URL: ws://localhost:8765
	// Attempt: 3
	// Unwrap: dial tcp: connection refused
}

// Example_daemonError constructs and inspects a DaemonError, which carries a
// numeric code, message, and optional data from the daemon.
func Example_daemonError() {
	// Simple daemon error.
	de := soothe.NewDaemonError(-32200, "loop not found")
	fmt.Println(de)
	fmt.Println("Code:", de.Code)
	fmt.Println("Message:", de.Message)

	// Daemon error with structured data.
	deWithData := &soothe.DaemonError{
		Code:    -32603,
		Message: "internal error",
		Data:    map[string]interface{}{"detail": "checkpoint corruption"},
	}
	fmt.Println(deWithData)
	// Output:
	// daemon error [-32200]: loop not found
	// Code: -32200
	// Message: loop not found
	// daemon error [-32603]: internal error (map[detail:checkpoint corruption])
}

// Example_timeoutError constructs a TimeoutError for a stalled RPC call.
func Example_timeoutError() {
	te := soothe.NewTimeoutError("daemon_status", "5s")
	fmt.Println(te)
	fmt.Println("Operation:", te.Operation)
	fmt.Println("Duration:", te.Duration)
	// Output:
	// timeout after 5s waiting for daemon_status
	// Operation: daemon_status
	// Duration: 5s
}

// Example_reconnectError constructs and inspects a ReconnectError, which wraps
// a failed reconnection attempt sequence.
func Example_reconnectError() {
	cause := fmt.Errorf("websocket: bad handshake")
	re := soothe.NewReconnectError("ws://localhost:8765", 10, cause)
	fmt.Println(re)
	fmt.Println("URL:", re.URL)
	fmt.Println("Attempts:", re.Attempts)
	fmt.Println("Unwrap:", errors.Unwrap(re))
	// Output:
	// reconnect to ws://localhost:8765 failed after 10 attempts: websocket: bad handshake
	// URL: ws://localhost:8765
	// Attempts: 10
	// Unwrap: websocket: bad handshake
}

// Example_staleLoopError constructs and inspects a StaleLoopError, returned by
// ReattachAndProbe when a loop accepts reattach but fails the liveness probe.
func Example_staleLoopError() {
	cause := fmt.Errorf("loop_get timeout")
	se := soothe.NewStaleLoopError("loop-abc-123", cause)
	fmt.Println(se)
	fmt.Println("LoopID:", se.LoopID)
	fmt.Println("Unwrap:", errors.Unwrap(se))
	// Output:
	// stale loop loop-abc-123: reattach accepted but liveness probe failed: loop_get timeout
	// LoopID: loop-abc-123
	// Unwrap: loop_get timeout
}

// Example_disconnectCause shows the clean/unclean disconnect cause constants
// and their String() representations.
func Example_disconnectCause() {
	fmt.Println(soothe.DisconnectUnclean.String()) // unclean
	fmt.Println(soothe.DisconnectClean.String())   // clean
	fmt.Println(int(soothe.DisconnectUnclean))     // 0
	fmt.Println(int(soothe.DisconnectClean))       // 1
	// Output:
	// unclean
	// clean
	// 0
	// 1
}

// Example_typeAssertDaemonError shows how to check if an error from a client
// method is a *DaemonError using errors.As, which traverses wrapped errors.
func Example_typeAssertDaemonError() {
	// Simulate a daemon error returned from a client method.
	err := &soothe.DaemonError{Code: -32200, Message: "loop not found"}

	var de *soothe.DaemonError
	if errors.As(err, &de) {
		fmt.Printf("daemon error code=%d message=%s\n", de.Code, de.Message)
	} else {
		fmt.Println("not a daemon error")
	}
	// Output:
	// daemon error code=-32200 message=loop not found
}

// Example_typeAssertStaleLoopError shows how to detect a *StaleLoopError from
// ReattachAndProbe to decide whether to fall back to a fresh bootstrap.
func Example_typeAssertStaleLoopError() {
	err := soothe.NewStaleLoopError("loop-xyz", fmt.Errorf("probe timeout"))

	var se *soothe.StaleLoopError
	if errors.As(err, &se) {
		fmt.Printf("stale loop %s — falling back to fresh bootstrap\n", se.LoopID)
	}
	// Output:
	// stale loop loop-xyz — falling back to fresh bootstrap
}

// Example_heartbeatError shows the HeartbeatError returned by
// HeartbeatTracker.WaitForAlive when the daemon does not become alive.
func Example_heartbeatError() {
	tracker := soothe.NewHeartbeatTracker()
	err := tracker.WaitForAlive(100 * time.Millisecond)
	if err != nil {
		fmt.Println("Got error:", err)
	}
	// Output:
	// Got error: timeout waiting for daemon to be alive (no heartbeat received)
}
