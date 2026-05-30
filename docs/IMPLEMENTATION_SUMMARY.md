# Go client — implementation notes

## Scope

The `soothe-client-go` package speaks the Soothe daemon **WebSocket** protocol with a **loop-centric** API (`loop_*` messages, `loop_id` fields, `BootstrapLoopSession`, etc.). Older **thread**-named WebSocket RPCs are not supported by this client.

## WebSocket coverage (high level)

1. **Loop lifecycle** — `loop_new`, `loop_list`, `loop_get`, `loop_tree`, `loop_prune`, `loop_delete`, `loop_reattach`, `loop_subscribe`, `loop_detach`, `loop_input`
2. **Streaming** — `event`, `status`, NDJSON frames, verbosity-related fields on subscribe
3. **Skills / models** — `skills_list`, `models_list`, `invoke_skill`
4. **Daemon** — `daemon_ready`, `daemon_status`, `daemon_shutdown`, `config_get`
5. **Input** — `loop_input` / `SendInput` with `WithLoopID`, autonomous options, attachments (see SDK parity)
6. **Structured commands** — `command_request` with `loop_id` (RFC-404 wrappers in `request.go`)
8. **Health** — optional heartbeat tracking (`HeartbeatTracker`, `loop_id` in heartbeat payloads)

## REST-only features

Some endpoints exist only on **HTTP REST** (not this package), for example richer health metrics or autopilot CRUD. Use a separate HTTP client if you need those.

## RPC command helpers

`CommandClear`, `CommandExit`, `CommandQuit`, `CommandDetach`, `CommandCancel`, `CommandMemory`, `CommandHistory`, `CommandReview`, `CommandPlan`, `CommandAutopilotDashboard`, etc., all send `command_request` with a **`loop_id`** scope where applicable.

## Documentation

- `README.md` — usage and limitations
- `docs/API_TEST_COVERAGE.md` — which tests cover which APIs
- `docs/heartbeat.md` — heartbeat tracker and `DaemonHealth.LoopID`

## Historical note

Earlier revisions of this repo described `thread_stats` and thread-named WebSocket APIs on the Go side. The daemon and client have moved to **loop** naming; statistics or CRUD that still live under legacy REST paths should be documented per the main Soothe HTTP API, not as `thread_stats` in this WebSocket client unless explicitly reintroduced in `protocol.go`.
