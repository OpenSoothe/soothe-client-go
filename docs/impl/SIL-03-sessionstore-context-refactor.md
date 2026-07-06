# SIL-03 — SessionStore context.Context refactor (post-v0.2.4, unreleased)

**Status:** unreleased (HEAD is `v0.2.4-5-g2ffdb6a` at time of writing)
**Breaking:** yes — `appkit.SessionStore` interface signature change
**Target tag:** `v0.2.5` (or `v0.3.0` if consumers prefer a major-minor bump for the break)

## Summary

All six methods of the `appkit.SessionStore` interface now take a
`context.Context` as their first parameter, so that cancellation, deadlines,
and trace spans can flow from the caller's request context down into the
persistence layer. Previously every method took only plain scalar args and had
no way to observe a cancelled turn or an expiring request budget.

The refactor was made in commit `2ffdb6a` (5 commits ahead of `v0.2.4`).

## Affected interface — `appkit.SessionStore`

File: `appkit/session_store.go`

| Method | Old signature | New signature |
|--------|---------------|---------------|
| `GetSession` | `(sessionID string) (*SessionEntry, error)` | `(ctx context.Context, sessionID string) (*SessionEntry, error)` |
| `CreateSession` | `(workspaceID, sessionID, loopID, sessionType string) error` | `(ctx context.Context, workspaceID, sessionID, loopID, sessionType string) error` |
| `UpdateLastUsed` | `(sessionID string) error` | `(ctx context.Context, sessionID string) error` |
| `IncrementResetCount` | `(sessionID string) error` | `(ctx context.Context, sessionID string) error` |
| `GetLoopIDForSession` | `(sessionID string) (loopID string, ok bool, err error)` | `(ctx context.Context, sessionID string) (loopID string, ok bool, err error)` |
| `AppendMessage` | `(sessionID string, message SessionMessage) error` | `(ctx context.Context, sessionID string, message SessionMessage) error` |

## Migration for implementers

Any type that implements `appkit.SessionStore` must add a `ctx context.Context`
first parameter to all six methods. The context should be honoured in the
storage backend (e.g. `pgx` `QueryRow(ctx, ...)`, Redis `Do(ctx, ...)`,
in-memory stores that do blocking I/O should `select` on `ctx.Done()`).

### Consumers already migrated

The two `SessionStore` implementations that ship in **mizar-airway** were
updated in lockstep with this refactor:

- `mizar-airway/internal/agent/mem_session_store.go` — `MemSessionStore`
  (in-memory, no-op on `ctx` but signature-compliant)
- `mizar-airway/internal/agent/pg_session_store.go` — `PGSessionStore`
  (Postgres via `pgx`; `ctx` is forwarded into `pgx` queries)

Callers inside the client itself (`appkit/pool.go`, `appkit/turn_runner.go`)
were updated in the same commit to thread the caller's `ctx` through to every
store call.

## Why context on the store?

- **Request-scoped cancellation:** a cancelled HTTP request or turn should
  short-circuit a pending `AppendMessage` or `CreateSession` write rather than
  running to completion against a dead request.
- **Deadlines:** `TurnRunner.Run` already wraps the turn in a
  `context.WithTimeout(ctx, cfg.QueryTimeout)`; that deadline now propagates to
  `persistResponse` / `persistFailed` instead of the store doing unbounded work.
- **Observability:** trace spans attached to `ctx` reach the DB driver, so
  store latency shows up in distributed traces without manual plumbing.

## Call-site map (context propagation)

### `appkit/pool.go` — `ConnectionPool.Acquire`

The `Acquire(ctx, sessionID, workspaceID, userID)` method passes `ctx` to:

- `p.store.UpdateLastUsed(ctx, sessionID)` — on the fast path (existing live
  connection reused), line ~178
- `p.store.GetLoopIDForSession(ctx, sessionID)` — via `p.loopIDFor(ctx, ...)`,
  line ~190 / ~348, to decide fresh bootstrap vs reattach
- `p.store.CreateSession(ctx, workspaceID, sessionID, loopID, "")` — after a
  successful fresh bootstrap (line ~203) and after a reattach-fail → fresh
  bootstrap fallback (line ~219)
- `p.store.UpdateLastUsed(ctx, sessionID)` — post-acquire stamp (line ~232)

### `appkit/turn_runner.go` — `TurnRunner`

- `persistResponse(ctx, sessionID, loopID, content, startedAt, completionEvent)`
  calls `r.store.AppendMessage(ctx, sessionID, msg)` (line ~310) with the
  turn-level context (which already carries the `QueryTimeout` deadline).
- `persistFailed(ctx, sessionID, loopID, err)` calls
  `r.store.AppendMessage(ctx, sessionID, msg)` (line ~327) on the error path.

Both `persist*` helpers short-circuit on `r.store == nil` so a nil store is
still valid for stateless deployments.

## Not in this refactor

- `IncrementResetCount` has no internal call-site in `pool.go` or
  `turn_runner.go` yet (it is reserved for the explicit reset path). It is
  included in the interface change for forward-compatibility so a future
  `Reset` caller can pass a context without another breaking change.
- `GetSession` likewise has no internal caller in the client; it is part of
  the interface for app-level session inspection. Signature updated for
  consistency.

## Release checklist (do before tagging `v0.2.5`)

- [ ] Remove the local `replace` directive in `mizar-airway/go.mod` once a
      published module at the new tag is available.
- [ ] `git tag v0.2.5` and `git push --tags`.
- [ ] Confirm `mizar-airway` `go.mod` `require` line bumps to the new tag and
      builds without the `replace`.
- [ ] (Optional, tracked separately) add a unit test for `ConnectWithRetries`
      covering the `maxRetries<=0` default, `retryDelay<=0` default, and
      `ctx.Done()` cancellation path — see SIL-04.
