# Soothe Daemon API Test Coverage Summary

## Test files (this repo)

| File | Role |
|------|------|
| `client_test.go` | Unit tests with mock WebSocket servers |
| `helpers_test.go` | Helper RPC tests (daemon status, config, skills) |
| `heartbeat_test.go` | Heartbeat tracker unit tests |
| `integration_test.go` | Core integration tests against a live daemon |
| `integration_loop_test.go` | Loop management and command RPC integration tests |
| `integration_image_test.go` | Image attachment integration tests |

## Loop management

| API | Typical test |
|-----|----------------|
| `LoopNew` / `SendLoopNew` | `integration_loop_test.go:TestIntegration_LoopNew` |
| `LoopList` / `SendLoopList` | `integration_loop_test.go:TestIntegration_LoopList` |
| `LoopGet` / `SendLoopGet` | `integration_loop_test.go:TestIntegration_LoopGet` |
| `LoopTree` / `SendLoopTree` | `integration_loop_test.go:TestIntegration_LoopTree` |
| `LoopPrune` / `SendLoopPrune` | `integration_loop_test.go:TestIntegration_LoopPrune` |
| `LoopDelete` / `SendLoopDelete` | `integration_loop_test.go:TestIntegration_LoopDelete` |
| `LoopReattach` / `SendLoopReattach` | `integration_loop_test.go:TestIntegration_LoopReattach` |
| `LoopSubscribe` / `SendLoopSubscribe` | `integration_loop_test.go:TestIntegration_LoopSubscribeDetach` |
| `LoopDetach` / `SendLoopDetach` | `integration_loop_test.go:TestIntegration_LoopSubscribeDetach` |
| `LoopInput` / `SendLoopInput` | `integration_loop_test.go:TestIntegration_LoopInput` |
| `BootstrapLoopSession` | `integration_test.go:TestIntegration_NewLoopCreation` |
| `SendInput` + `WithLoopID` | `integration_test.go:TestIntegration_InputMessage`, `client_test.go` |

## Skills, models, daemon

| API | Typical test |
|-----|----------------|
| `ListSkills` / `SendSkillsList` | `integration_test.go:TestIntegration_SkillsList` |
| `ListModels` / `SendModelsList` | `integration_test.go:TestIntegration_ModelsList` |
| `InvokeSkill` | `client_test.go:TestClient_InvokeSkill` |
| `SendDaemonReady` / `WaitForDaemonReady` | `integration_test.go:TestIntegration_DaemonReady`, `client_test.go:TestClient_WaitForDaemonReady` |
| `CheckDaemonStatus` / `RequestDaemonShutdown` | `helpers_test.go`, `integration_test.go:TestIntegration_CheckDaemonStatus` |
| `SendDetach` | `integration_test.go:TestIntegration_SendDetach` |

## Commands

| API | Typical test |
|-----|----------------|
| `CommandRequest` / `SendCommandRequest` | `integration_loop_test.go:TestIntegration_CommandRequest` |
| `SendCommand` (slash) | `client_test.go:TestClient_SendCommand` |

## Protocol helpers

| Feature | Typical test |
|---------|----------------|
| `DecodeMessage` / NDJSON | `helpers_test.go`, `client_test.go:TestClient_NDJSONReceive` |
| `ExtractSootheLoopID` | Used across integration tests |
| `EventMessage.LoopAIMessage` | `client_test.go:TestClient_ReceiveMessages_LoopAIMessageEvent` |

## Running tests

```bash
# No daemon
go test -short ./...

# Live daemon at ws://localhost:8765 (or URL in config)
go test -timeout 120s ./...
```

```bash
go test -v -run 'TestIntegration_Loop' ./...
go test -v -run 'TestClient_' ./...
```

## Notes

- Integration tests skip or log when the daemon is unreachable (`testing.Short()` or connection failure).
- Legacy **thread** WebSocket message types (`thread_*`, `subscribe_thread`, etc.) are not part of the current daemon protocol; this client targets **loop** messages only.
