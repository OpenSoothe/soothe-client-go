# soothe-client-go

WebSocket client in Go for soothe-daemon

**Package:** https://github.com/mirasoth/soothe-client-go

## API Coverage

This client implements the WebSocket protocol for Soothe daemon, providing full access to:

- Loop lifecycle APIs (new, list, get, tree, prune, delete, reattach, subscribe, detach, input)
- Skills/models discovery APIs
- Daemon control APIs (status, shutdown, health monitoring)
- Input/command APIs with autonomous mode support
- Event streaming with verbosity filtering
- Heartbeat tracking for daemon health monitoring

### Limitations

**Autopilot HTTP endpoints are NOT available via WebSocket** - they require HTTP REST API:

- `/api/v1/autopilot/status` - Get autopilot state
- `/api/v1/autopilot/goals` - List/submit/approve/reject goals
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

```
soothe-client-go/
├── client.go, protocol.go, session.go, …  — Layer 0 transport
├── command_client.go, stream_terminal.go  — CommandClient + turn helpers
└── appkit/                                — DaemonSession, pool, TurnRunner, …
```

## Usage

```go
import (
    "context"
    "fmt"

    soothe "github.com/mirasoth/soothe-client-go"
    "github.com/mirasoth/soothe-client-go/appkit"
)

ctx := context.Background()
session := appkit.NewDaemonSession("ws://127.0.0.1:8765", nil)
defer session.Close()

if _, err := session.Connect(ctx, ""); err != nil {
    panic(err)
}
if err := session.SendTurn(ctx, "Summarize this repo", nil); err != nil {
    panic(err)
}
chunks, errCh := session.IterTurnChunks(ctx, 0)
for chunk := range chunks {
    fmt.Println(chunk.Mode, chunk.Data)
}
if err := <-errCh; err != nil {
    panic(err)
}
```

## What you get

| Need | Use |
|------|-----|
| One conversation, stream replies | `appkit.DaemonSession` |
| Jobs / cron one-shots | `soothe.CommandClient` |
| Raw WebSocket / custom RPCs | `soothe.Client` |
| Many users / HTTP backend | `appkit.ConnectionPool` + `TurnRunner` |

### Limitations

**Autopilot HTTP endpoints are NOT available via WebSocket** - they require HTTP REST API:

- `/api/v1/autopilot/status` - Get autopilot state
- `/api/v1/autopilot/goals` - List/submit/approve/reject goals
- `/api/v1/autopilot/wake` / `/api/v1/autopilot/dream` - Mode transitions

The daemon only exposes these through HTTP REST transport. To use autopilot features in Go via REST, implement an HTTP client separately (not included in this package). WebSocket job/cron RPCs use `CommandClient` or `Client` job helpers.

## appkit — SessionStore & ConnectionPool

The `appkit` subpackage provides higher-level building blocks for apps that
pool daemon connections and persist session↔loop mappings:

- `appkit.ConnectionPool` — manages a pool of daemon connections, one active
  per session; bootstraps a fresh loop (`loop_new` + `subscribe`) or reattaches
  an existing one (`loop_reattach` + `subscribe`).
- `appkit.TurnRunner` — executes one query turn end-to-end (acquire pooled
  connection, send `loop_input`, consume the event stream, resolve the
  deliverable, persist the reply).
- `appkit.SessionStore` — the persistence seam between appkit and the app's
  storage backend. Implementations must be safe for concurrent use.

### SessionStore interface (context-aware, post-v0.2.4)

**Breaking change:** as of the post-v0.2.4 refactor (see
`docs/impl/SIL-03-sessionstore-context-refactor.md`), all six `SessionStore`
methods take a `context.Context` as their first parameter. This lets request
cancellation, deadlines, and trace spans flow from the caller's context down
into the storage backend.

```go
import (
    "context"
    "github.com/mirasoth/soothe-client-go/appkit"
)

type MyStore struct { /* ... */ }

// All six methods gain a ctx context.Context first parameter:
func (s *MyStore) GetSession(ctx context.Context, sessionID string) (*appkit.SessionEntry, error) { /* ... */ }
func (s *MyStore) CreateSession(ctx context.Context, workspaceID, sessionID, loopID, sessionType string) error { /* ... */ }
func (s *MyStore) UpdateLastUsed(ctx context.Context, sessionID string) error { /* ... */ }
func (s *MyStore) IncrementResetCount(ctx context.Context, sessionID string) error { /* ... */ }
func (s *MyStore) GetLoopIDForSession(ctx context.Context, sessionID string) (loopID string, ok bool, err error) { /* ... */ }
func (s *MyStore) AppendMessage(ctx context.Context, sessionID string, message appkit.SessionMessage) error { /* ... */ }
```

The context should be honoured in the storage backend (e.g. forward `ctx` to
`pgx`/Redis calls; `select` on `ctx.Done()` for blocking in-memory stores).
`ConnectionPool.Acquire` and `TurnRunner`'s `persistResponse`/`persistFailed`
were updated in lockstep to thread the caller's `ctx` through to every store
call.

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

This client implements the same protocol-1 contract as `soothe-client-python` and `@mirasoth/soothe-client` (see RFC-629 / IG-662).