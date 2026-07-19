# Progressive examples

Mirror the Python `examples/01`–`06` ladder. These Go tests use the in-process
mock daemon (offline). For a live daemon, set `SOOTHE_WS_URL` and run the
integration suite instead.

| Script | What it shows |
|--------|----------------|
| `01_hello` | Connect + bootstrap loop |
| `02_stream_turn` | `DaemonSession` send + iterate chunks |
| `03_text_completion` | `intent_hint=text_completion` |
| `04_multi_turn` | Follow-ups on the same loop |
| `05_pool_service` | `ConnectionPool` stats (`TurnRunner` uses `TurnBoundary` for turn end) |
| `06_jobs` | `CommandClient` job create/status/cancel |

```bash
cd client/go
go test ./examples/progressive/ -count=1
```
