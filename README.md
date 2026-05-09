# soothe-client-go

WebSocket client in Go for soothe-daemon

**Package:** https://github.com/mirasoth/soothe-client-go

## API Coverage

This client implements the WebSocket protocol for Soothe daemon, providing full access to:

- Loop lifecycle APIs (RFC-503, RFC-504: new, list, get, tree, prune, delete, reattach, subscribe, detach, input)
- Skills/models discovery APIs
- Daemon control APIs (status, shutdown, health monitoring)
- Input/command APIs with autonomous mode support
- Event streaming with verbosity filtering
- Heartbeat tracking for daemon health monitoring

### Limitations

**Autopilot endpoints (RFC-204) are NOT available via WebSocket** - they require HTTP REST API:

- `/api/v1/autopilot/status` - Get autopilot state
- `/api/v1/autopilot/goals` - List/submit/approve/reject goals
- `/api/v1/autopilot/inbox` - View pending tasks
- `/api/v1/autopilot/wake` / `/api/v1/autopilot/dream` - Mode transitions

The daemon only exposes these through HTTP REST transport (`http_rest.py`). To use autopilot features in Go, you would need to implement an HTTP REST client separately (not included in this package).

### WebSocket vs HTTP REST

**WebSocket** (this client):
- Real-time event streaming
- Interactive query execution
- Loop lifecycle operations
- Bidirectional communication for autonomous agents

**HTTP REST** (not implemented):
- Autopilot goal management
- Enhanced health metrics with queue depths
- Loop or run statistics and other CRUD may be available only on HTTP REST (not implemented in this WebSocket client)
- RESTful CRUD operations

## Package Structure

The client uses a flat package structure with all code in the root `soothe` package:

```
soothe-client-go/
├── client.go           - Core Client struct and connection management
├── send_methods.go     - High-level Send* API methods
├── request.go          - Request-response pattern methods
├── verbosity.go        - Verbosity types for event filtering
├── session.go          - Bootstrap helpers for loop session (daemon_ready, loop_new, loop_subscribe)
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
if err := client.SendInput(ctx, "Hello", soothe.WithLoopID("loop-123")); err != nil {
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