# Go client — implementation notes

## Scope

The `soothe-client-go` package speaks the Soothe daemon **WebSocket** protocol with a **loop-centric** API (`loop_*` messages, `loop_id` fields, `BootstrapLoopSession`, etc.). Older **thread**-named WebSocket RPCs are not supported by this client.

## WebSocket coverage (high level)

1. **Loop lifecycle** — `loop_new`, `loop_list`, `loop_get`, `loop_tree`, `loop_prune`, `loop_delete`, `loop_reattach`, `loop_subscribe`, `loop_detach`, `loop_input`
2. **Streaming** — `event`, `status`, NDJSON frames, verbosity-related fields on subscribe
3. **Skills / models** — `skills_list`, `models_list`, `invoke_skill`
4. **Daemon** — `daemon_ready`, `daemon_status`, `daemon_shutdown`, `config_get`
5. **Input** — `loop_input` / `SendInput` with `WithLoopID`, autonomous options, attachments (see SDK parity)
6. **Structured commands** — `command_request` with `loop_id` (wrappers in `request.go`)
8. **Health** — optional heartbeat tracking (`HeartbeatTracker`, `loop_id` in heartbeat payloads)

## REST-only features

Some endpoints exist only on **HTTP REST** (not this package), for example richer health metrics or autopilot CRUD. Use a separate HTTP client if you need those.

## RPC command helpers

`CommandClear`, `CommandExit`, `CommandQuit`, `CommandDetach`, `CommandCancel`, `CommandMemory`, `CommandHistory`, `CommandReview`, `CommandPlan`, `CommandAutopilotDashboard`, etc., all send `command_request` with a **`loop_id`** scope where applicable.

## Documentation

- `README.md` — usage and limitations
- `docs/API_TEST_COVERAGE.md` — which tests cover which APIs
- `docs/heartbeat.md` — heartbeat tracker and `DaemonHealth.LoopID`
- `docs/impl/SIL-03-sessionstore-context-refactor.md` — breaking `SessionStore` context.Context refactor (post-v0.2.4)
- `docs/impl/SIL-04-appkit-turn-lifecycle.md` — idle timeout, status-idle, compaction, soft stream-close

## Context propagation (post-v0.2.4 refactor)

As of commit `2ffdb6a` (unreleased; HEAD is `v0.2.4-5-g2ffdb6a`), the
`appkit.SessionStore` interface and its internal callers thread a
`context.Context` from the request boundary down into the persistence layer.
This is a **breaking** interface change — see
`docs/impl/SIL-03-sessionstore-context-refactor.md` for the full method-by-
method signature table and migration notes.

### `appkit/pool.go` — `ConnectionPool.Acquire`

`Acquire(ctx, sessionID, workspaceID, userID)` propagates `ctx` to every store
call it makes:

- **Fast path (live connection reused):** `p.store.UpdateLastUsed(ctx, sessionID)` — stamps the last-used timestamp on the reused slot.
- **Loop lookup:** `p.loopIDFor(ctx, sessionID)` → `p.store.GetLoopIDForSession(ctx, sessionID)` — decides between a fresh `loop_new` bootstrap and a `loop_reattach` reattach.
- **Fresh bootstrap:** after `p.bootstrapNew(ctx, ...)` returns a new `loopID`, `p.store.CreateSession(ctx, workspaceID, sessionID, loopID, "")` persists the new session↔loop mapping.
- **Reattach-fail fallback:** if `p.resumeAndReattach(ctx, ...)` fails, the pool falls back to a fresh bootstrap and calls `p.store.CreateSession(ctx, ...)` again.
- **Post-acquire stamp:** `p.store.UpdateLastUsed(ctx, sessionID)` on the newly acquired slot.
- **Pool exhaustion / cancellation:** the `select` on `p.pool` also watches `<-ctx.Done()` so a cancelled caller gets `ctx.Err()` instead of blocking indefinitely for a free slot.

### `appkit/turn_runner.go` — `TurnRunner`

`TurnRunner.Run` wraps the turn in `context.WithTimeout(ctx, cfg.QueryTimeout)`
(default 30m); that deadline now reaches the store via two persist helpers:

- **`persistResponse(ctx, ...)`** → `r.store.AppendMessage(ctx, sessionID, msg)` — writes the assistant reply row with `Role="assistant"` and metadata (`started_at`, `completed_at`, `duration_ms`, `status`, `completion_event`, `deliverable`). Runs after a successful turn.
- **`persistFailed(ctx, ...)`** → `r.store.AppendMessage(ctx, sessionID, msg)` — writes an error row with `Role="error"` and `status="failed"`. Runs on the error path.

Both helpers short-circuit on `r.store == nil` (stateless deployments are
still valid). The context they receive is the turn-level context, so a turn
that times out will also cancel the pending `AppendMessage` write rather than
letting it run unbounded against a dead request.

### Why this matters

Before the refactor, `SessionStore` methods had no `context.Context`, so:

- a cancelled HTTP request or turn would not short-circuit a pending
  `CreateSession` or `AppendMessage` write;
- the `TurnRunner` query-timeout deadline could not reach the DB driver;
- trace spans attached to the caller's context did not reach the store.

All three are fixed by the context parameter. Application `SessionStore`
implementations should accept and honor `ctx` the same way.

### Not yet wired

- `IncrementResetCount` and `GetSession` have no internal caller in the client
  yet (reserved for app-level reset/inspection). Their signatures were updated
  for consistency so a future caller can pass a context without another
  breaking change.
- `ConnectWithRetries` (`session.go`) is added and wired but not yet unit-
  tested — tracked separately.

## Historical note

Earlier revisions of this repo described `thread_stats` and thread-named WebSocket APIs on the Go side. The daemon and client have moved to **loop** naming; statistics or CRUD that still live under legacy REST paths should be documented per the main Soothe HTTP API, not as `thread_stats` in this WebSocket client unless explicitly reintroduced in `protocol.go`.
