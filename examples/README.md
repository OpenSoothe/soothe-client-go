# Examples

Runnable example tests that exercise the public API against an in-process mock
daemon (no live `soothed` required).

```bash
# from client/go
go test ./examples/ -count=1
go test ./examples/progressive/ -count=1
go test ./examples/appkit/ -count=1
```

| Path | What it shows |
|------|----------------|
| [`progressive/`](progressive/) | Ladder 01–06: hello → `DaemonSession` stream → multi-turn → pool → `CommandClient` jobs |
| [`appkit/`](appkit/) | Pool, `TurnRunner`, classifier, query gate, SSE |
| `connection_example_test.go` | Raw `Client` connect / bootstrap / retries |
| `job_cron_example_test.go` | Jobs and cron RPCs |
| `loop_management_example_test.go` | Loop list / get / tree / prune / delete |
| `commands_example_test.go` | Slash / structured commands |
| `skills_models_example_test.go` | Skills and models discovery |
| `daemon_control_example_test.go` | Daemon status / shutdown / config |
| `input_options_example_test.go` | `loop_input` options and attachments |
| `auth_example_test.go` | Auth handshake helpers |
| `verbosity_example_test.go` | Verbosity filtering |
| `heartbeat_example_test.go` | Heartbeat tracker |
| `protocol_example_test.go` | Envelope helpers |
| `errors_example_test.go` | Typed errors |

For a live daemon, point integration tests at `ws://127.0.0.1:8765` (or
`SOOTHE_DAEMON_URL`) and run `go test` without `-short`.
