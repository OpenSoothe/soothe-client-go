# Heartbeat Tracking

The Go client now supports heartbeat tracking to monitor daemon aliveness and prevent timeouts during long operations.

## Overview

The Soothe daemon broadcasts heartbeat events every 5 seconds when actively processing queries. The Go client can track these heartbeats to:

1. **Monitor daemon health** - Know if the daemon is alive and responding
2. **Detect daemon state** - Distinguish between "idle" and "running" states
3. **Prevent timeouts** - Avoid disconnecting during long LLM operations
4. **Track processing thread** - Know which thread the daemon is currently working on

## Quick Start

### Enable Heartbeat Tracking

```go
// Create client with automatic heartbeat tracking
client := soothe.NewClientWithHeartbeat("ws://localhost:8765", nil)

// Or enable it on an existing client
client := soothe.NewClient("ws://localhost:8765", nil)
client.EnableHeartbeatTracking()
```

### Check Daemon Health

```go
// Get current daemon health status
health := client.GetDaemonHealth()
if health != nil {
    fmt.Printf("Daemon alive: %v\n", health.IsAlive)
    fmt.Printf("Daemon state: %s\n", health.State) // "running" or "idle"
    fmt.Printf("Last heartbeat: %v\n", health.LastHeartbeat)
    fmt.Printf("Processing thread: %s\n", health.ThreadID)
}

// Quick check if daemon is alive
if client.IsDaemonAlive() {
    fmt.Println("Daemon is alive and responding")
}
```

### Monitor State Changes

```go
tracker := client.GetHeartbeatTracker()
if tracker != nil {
    // Check if daemon is actively processing
    if tracker.IsProcessing() {
        fmt.Println("Daemon is processing a query")
        fmt.Printf("Thread ID: %s\n", tracker.GetThreadID())
    }

    // Check if daemon is idle
    if tracker.IsIdle() {
        fmt.Println("Daemon is idle, ready for new queries")
    }

    // Get current state
    state := tracker.GetState() // "running" or "idle"
}
```

## Architecture

### Heartbeat Event Structure

The daemon sends heartbeat events with the following structure:

```json
{
  "type": "event",
  "thread_id": "thread-123",
  "data": {
    "type": "soothe.system.daemon.heartbeat",
    "thread_id": "thread-123",
    "timestamp": "2024-01-01T00:00:00Z",
    "state": "running"  // or "idle"
  }
}
```

### HeartbeatTracker

The `HeartbeatTracker` type manages heartbeat state:

- **Thread-safe**: Safe for concurrent access from multiple goroutines
- **Automatic processing**: Heartbeats are automatically processed when receiving messages
- **Grace period**: 20-second initial grace period for cold-start scenarios
- **Alive threshold**: Configurable threshold (default: 15 seconds) to determine if daemon is alive

### Alive Detection Logic

The daemon is considered alive if:

1. A heartbeat was received within the alive threshold (default 15 seconds)
2. OR we're in the initial grace period (first 20 seconds after tracking starts)

The 15-second threshold allows for 2 missed heartbeats at the daemon's 5-second interval.

## Usage Patterns

### Pattern 1: Wait for Daemon to Be Alive

```go
client := soothe.NewClientWithHeartbeat("ws://localhost:8765", nil)
// ... connect and start receiving messages ...

tracker := client.GetHeartbeatTracker()
if tracker != nil {
    // Wait up to 30 seconds for daemon to be alive
    if err := tracker.WaitForAlive(30 * time.Second); err != nil {
        return fmt.Errorf("daemon not alive: %w", err)
    }
}

// Proceed with normal operations
loopID, err := soothe.BootstrapLoopSession(ctx, client, "", "/workspace", nil)
```

### Pattern 2: Monitor During Long Operations

```go
client := soothe.NewClientWithHeartbeat("ws://localhost:8765", nil)
// ... setup connection and loop (BootstrapLoopSession) ...

// Send a potentially long-running query
client.SendInput(ctx, "Analyze this large codebase", soothe.WithLoopID(loopID))

// Process events while monitoring health
tracker := client.GetHeartbeatTracker()
for msg := range eventCh {
    // Check daemon health during processing
    if tracker.IsProcessing() {
        fmt.Println("Daemon is still processing, no timeout")
    }

    // Handle events
    // ...
}
```

### Pattern 3: Custom Alive Threshold

```go
// Use a custom threshold for slower networks or longer operations
client := soothe.NewClient("ws://localhost:8765", nil)
client.EnableHeartbeatTrackingWithThreshold(25 * time.Second)

// Daemon now considered alive if heartbeat received within 25 seconds
```

### Pattern 4: React to State Changes

```go
tracker := client.GetHeartbeatTracker()
lastState := ""

for msg := range eventCh {
    currentState := tracker.GetState()
    if currentState != lastState {
        fmt.Printf("State changed: %s -> %s\n", lastState, currentState)

        if currentState == "idle" {
            // Daemon finished processing
            fmt.Println("Ready for next query")
        }

        lastState = currentState
    }
}
```

## Integration with Existing Code

### Automatic Processing

When heartbeat tracking is enabled, `ReceiveMessages` automatically processes heartbeat events:

```go
client := soothe.NewClientWithHeartbeat("ws://localhost:8765", nil)
client.Connect(ctx)

eventCh, err := client.ReceiveMessages(ctx)
// Heartbeat events are automatically processed before being forwarded to eventCh
```

### Manual Processing

You can also manually process heartbeat events:

```go
tracker := client.GetHeartbeatTracker()

// Process a raw event
for msg := range eventCh {
    if m, ok := msg.(map[string]interface{}); ok {
        if tracker.ProcessHeartbeatEvent(m) {
            fmt.Println("Heartbeat processed")
        }
    }
}
```

### Reset on Reconnection

When reconnecting, reset the tracker to clear stale state:

```go
client.Close()

// Reconnect
client.Connect(ctx)

// Reset tracker for fresh start
tracker := client.GetHeartbeatTracker()
if tracker != nil {
    tracker.Reset()
}
```

## API Reference

### Client Methods

```go
// Enable heartbeat tracking
client.EnableHeartbeatTracking()
client.EnableHeartbeatTrackingWithThreshold(threshold time.Duration)

// Disable tracking
client.DisableHeartbeatTracking()

// Get tracker
tracker := client.GetHeartbeatTracker()

// Get health status
health := client.GetDaemonHealth()
isAlive := client.IsDaemonAlive()
```

### HeartbeatTracker Methods

```go
// State queries
health := tracker.GetHealth()
state := tracker.GetState()
threadID := tracker.GetThreadID()
lastHB := tracker.GetLastHeartbeat()

// Boolean checks
isAlive := health.IsAlive
isProcessing := tracker.IsProcessing()
isIdle := tracker.IsIdle()

// Updates
tracker.Update(heartbeatData)
tracker.ProcessHeartbeatEvent(event)
tracker.Reset()
tracker.SetAliveThreshold(threshold)

// Waiting
tracker.WaitForAlive(timeout time.Duration) error
```

### DaemonHealth Fields

```go
type DaemonHealth struct {
    LastHeartbeat time.Time  // Last heartbeat timestamp
    State         string     // "running" or "idle"
    ThreadID      string     // Current thread being processed
    IsAlive       bool       // True if daemon is alive
}
```

## Event Constants

The heartbeat event type is defined in `events.go`:

```go
const EventDaemonHeartbeat = "soothe.system.daemon.heartbeat"
```

## Best Practices

### 1. Enable Tracking for Long Operations

Always enable heartbeat tracking when you expect queries to take longer than 20 seconds:

```go
// Good for code analysis, research tasks, etc.
client := soothe.NewClientWithHeartbeat(url, nil)
```

### 2. Check Health Before Critical Operations

```go
if !client.IsDaemonAlive() {
    return errors.New("daemon not responding")
}

// Proceed with critical operation
```

### 3. Use Appropriate Threshold

- Default (15s): Good for most cases
- Longer (25-30s): For slow networks or complex operations
- Shorter (5-10s): For strict monitoring (may give false negatives)

### 4. Reset on Reconnection

```go
client.Close()
time.Sleep(1 * time.Second)
client.Connect(ctx)

if tracker := client.GetHeartbeatTracker(); tracker != nil {
    tracker.Reset()
}
```

### 5. Monitor During Event Processing

```go
for {
    select {
    case msg := <-eventCh:
        // Process message

        // Check health periodically
        if !client.IsDaemonAlive() {
            log.Warn("Daemon health degraded")
        }
    case <-time.After(30 * time.Second):
        // Timeout check
        if health := client.GetDaemonHealth(); health != nil {
            log.Info("Daemon state: %s, alive: %v", health.State, health.IsAlive)
        }
    }
}
```

## Implementation Details

### Thread Safety

All tracker methods are thread-safe:

- Uses `sync.RWMutex` for concurrent access
- Safe to call from multiple goroutines
- No race conditions during updates

### Grace Period

The 20-second grace period handles:

- Cold-start scenarios (daemon just started)
- Connection establishment
- Initial handshake before first heartbeat

### Alive Threshold

The alive threshold logic:

```go
// Alive if heartbeat within threshold OR in grace period
isAlive := time.Since(lastHeartbeat) < threshold ||
           time.Since(startTime) < gracePeriod
```

## Testing

Run the heartbeat tests:

```bash
go test -v ./heartbeat_test.go ./heartbeat.go
```

All tests pass and cover:

- Tracker creation and initialization
- Update processing
- State queries (IsProcessing, IsIdle)
- Event processing
- Reset functionality
- Concurrent access
- Custom thresholds
- Error handling

## See Also

- `heartbeat.go` - Implementation
- `heartbeat_test.go` - Unit tests
- `client.go` - Client integration
- `examples/heartbeat_example_test.go` - Usage examples