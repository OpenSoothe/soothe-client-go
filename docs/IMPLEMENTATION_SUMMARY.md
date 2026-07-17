# Go client — implementation notes

## Scope

The `soothe-client-go` package speaks the Soothe daemon **WebSocket** protocol
with a **loop-centric** API. Public entry points:

| Need | Entry |
|------|--------|
| One conversation, stream turns | `appkit.DaemonSession` |
| Jobs / cron one-shots | `CommandClient` |
| Raw transport / custom RPCs | `Client` |
| Multi-user backends | `appkit.ConnectionPool` + `TurnRunner` |

## WebSocket coverage (high level)

1. **Loop lifecycle** — `loop_new`, `loop_list`, `loop_get`, `loop_tree`, `loop_prune`, `loop_delete`, `loop_reattach`, `loop_subscribe`, `loop_detach`, `loop_input`
2. **Streaming** — `event`, `status`, NDJSON frames, verbosity on subscribe, `delivery_ack` on terminal frames
3. **Skills / models** — `skills_list`, `models_list`, `invoke_skill`
4. **Daemon** — `daemon_ready`, `daemon_status`, `daemon_shutdown`, `config_get`
5. **Input** — `loop_input` / `SendInput` with `WithLoopID`, autonomous options, attachments
6. **Jobs / cron / autopilot** — `job_*`, `cron_*`, `autopilot_*` via `CommandClient` or `Client` helpers (single `RequestResponse`, no double-send)
7. **Structured commands** — `command_request` with `loop_id`
8. **Health** — optional heartbeat tracking (`HeartbeatTracker`)

## Autopilot vs jobs

- **`job_*`** — preferred job lifecycle (create / status / pause / resume / cancel / dag / guidance)
- **`autopilot_*`** — goal-level control (status, submit, list/get/cancel goals, wake/dream/resume) plus root-job listing
- **`AutopilotSubscribe`** — long-lived worker event stream (`subscribe` / `autopilot_events`); not for ephemeral `CommandClient`

## Documentation

- `README.md` — quick start and API tiers
- `examples/README.md` — example index
- `docs/API_TEST_COVERAGE.md` — which tests cover which APIs
- `docs/heartbeat.md` — heartbeat tracker and `DaemonHealth.LoopID`
- `docs/impl/sessionstore-context.md` — `SessionStore` context.Context refactor
- `docs/impl/turn-lifecycle.md` — idle timeout, status-idle, compaction, soft stream-close

## Context propagation (`SessionStore`)

`appkit.SessionStore` methods take `context.Context` as the first parameter so
request cancellation, deadlines, and trace spans reach the storage backend.
`ConnectionPool.Acquire` and `TurnRunner` persist helpers thread the caller's
`ctx` through every store call. See
[`docs/impl/sessionstore-context.md`](impl/sessionstore-context.md).
