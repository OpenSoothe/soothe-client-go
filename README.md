# soothe-client-go

Talk to a running **soothe-daemon** over WebSocket — send prompts, stream agent
turns, run jobs.

```bash
go get github.com/mirasoth/soothe-client-go@latest
```

Requires a local daemon (default `ws://127.0.0.1:8765`).

## Quick start

```go
package main

import (
	"context"
	"fmt"

	"github.com/mirasoth/soothe-client-go/appkit"
)

func main() {
	ctx := context.Background()
	session := appkit.NewDaemonSession("ws://127.0.0.1:8765", nil)
	defer session.Close()

	if _, err := session.Connect(ctx, ""); err != nil {
		panic(err)
	}
	if err := session.SendTurn(ctx, "Summarize this in one sentence: agents need tools.", nil); err != nil {
		panic(err)
	}
	chunks, errCh := session.IterTurnChunks(ctx, 0)
	for chunk := range chunks {
		fmt.Println(chunk.Mode, chunk.Data)
	}
	if err := <-errCh; err != nil {
		panic(err)
	}
}
```

More patterns: [`examples/`](examples/) (hello → streaming → multi-turn → pool → jobs).

## What you get

| Need | Use |
|------|-----|
| One conversation, stream replies | `appkit.DaemonSession` |
| Jobs / cron one-shots | `soothe.CommandClient` |
| Raw WebSocket / custom RPCs | `soothe.Client` |
| Many users / HTTP backend | `appkit.ConnectionPool` + `TurnRunner` |

`DaemonSession` uses a dual-socket layout (stream + RPC sidecar): peels leftover
prior-goal terminals, ignores premature `soothe.stream.end` until the turn has
progress, drains a short post-idle window, and sends `delivery_ack` on terminal
frames for daemon drain gating.

```go
import (
	"time"
	soothe "github.com/mirasoth/soothe-client-go"
)

cc := soothe.NewCommandClient("ws://127.0.0.1:8765", 30*time.Second)
created, err := cc.JobCreate(ctx, "Echo: smoke job", "/tmp/workspace")
// … JobStatus / JobCancel / CronAdd / CronList …
```

## Package layout

```
soothe-client-go/
├── client.go, protocol.go, session.go, …  — transport + Client RPCs
├── command_client.go, stream_terminal.go  — CommandClient + turn helpers
└── appkit/                                — DaemonSession, pool, TurnRunner, …
```

## appkit — pool & TurnRunner

Product backends that map chat sessions to daemon loops use `ConnectionPool` +
`QueryGate` + `TurnRunner` + `EventClassifier` + `SessionStore`.

| Knob | Default | Notes |
|------|---------|--------|
| `IdleTimeout` | off | Silence watchdog between events |
| `MinIdleTimeoutWithAttachments` | off | Floor when attachments are present |
| `OnIdleTimeout` / `OnQueryTimeout` / `OnStreamClose` | fail | Or soft-complete |
| `CompactAttachmentsBeforeSend` | false | Optional image downscale |
| `TreatStatusIdleAsComplete` (classifier) | false | Opt-in idle deliverable |

`SessionStore` methods take `context.Context` as the first parameter so
cancellation and deadlines reach the storage backend. See
[`docs/impl/sessionstore-context.md`](docs/impl/sessionstore-context.md) and
[`docs/impl/turn-lifecycle.md`](docs/impl/turn-lifecycle.md).

## Limitations

**Autopilot HTTP endpoints are not available via WebSocket** — they need the
daemon's HTTP REST API (`/api/v1/autopilot/...`). This package speaks WebSocket
only. Prefer `CommandClient` for job/cron one-shots so they do not share a
streaming socket.

## Develop

```bash
go test ./...                       # unit + example tests (mock daemon)
go test ./examples/progressive/     # 01–06 ladder (offline)
go test -short ./...                # skip live-daemon integration
```

## Compatibility

Same protocol-1 WebSocket contract as `soothe-client-python` and
`@mirasoth/soothe-client`.

## License

MIT — see [LICENSE](./LICENSE).
