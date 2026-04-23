# soothe-client-go

WebSocket client in Go for soothe-daemon

**Package:** https://github.com/mirasoth/soothe-client-go

## Package Structure

The client uses a flat package structure with all code in the root `soothe` package:

```
soothe-client-go/
├── client.go           - Core Client struct and connection management
├── send_methods.go     - High-level Send* API methods
├── request.go          - Request-response pattern methods
├── verbosity.go        - Verbosity types for event filtering
├── session.go          - Bootstrap helpers for thread creation/resumption
├── helpers.go          - RPC convenience functions
├── config.go           - Client configuration
├── protocol.go         - Wire protocol message types
├── events.go           - Event namespace constants and classification
├── errors.go           - Custom error types
```

## Usage

```go
import "github.com/mirasoth/soothe-client-go"

// Create client
client := soothe.NewClient("ws://localhost:8080", nil)

// Connect
if err := client.Connect(ctx); err != nil {
    log.Fatal(err)
}

// Wait for daemon ready
if _, err := client.WaitForDaemonReady(10*time.Second); err != nil {
    log.Fatal(err)
}

// Send input
if err := client.SendInput(ctx, "Hello", soothe.WithThreadID("thread-123")); err != nil {
    log.Fatal(err)
}

// Receive events
ch, err := client.ReceiveMessages(ctx)
for msg := range ch {
    // Process message
}
```

## Verbosity

The package defines `VerbosityLevel` and `VerbosityTier` types for event filtering:

```go
// Check if event should be shown at current verbosity
tier := soothe.TierNormal
verbosity := soothe.VerbosityNormal
if soothe.ShouldShow(tier, verbosity) {
    // Display event
}
```

## Compatibility

This client implements the same protocol as the Python `soothe-sdk` and mirrors its API structure.