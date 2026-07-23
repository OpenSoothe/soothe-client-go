# Changelog

All notable changes to `soothe-client-go` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.4.7] - 2026-07-23

### Fixed
- `TurnRunner` ignores stale pooled `status.idle` / `stopped` / turn-scoped `stream.end` until the current turn arms (post-send `running` or assistant content), preventing leftover end markers from ending the next turn early

### Changed
- Handshake `ClientVersion` reports `0.4.7`

## [0.4.6] - 2026-07-23

### Fixed
- `ConnectionPool.startReader` detaches from the Acquire/request context via `context.WithoutCancel`, so multi-turn reuse no longer inherits a cancelled `ReceiveMessages` stream
- `Acquire` rebuilds the slot when the event stream is dead even if the WebSocket still looks connected
- `InputMessageForLoop` emits a protocol-1 `loop_input` notification envelope (parity with `SendInput`); legacy top-level `{"type":"loop_input"}` was rejected by soothed ≥0.9
- `ResolveDeliverableFinalContent` falls back to accumulated streamed text when the deliverable event has empty content (StrangeLoop early-complete)

### Changed
- Handshake `ClientVersion` reports `0.4.6`

## [0.4.5] - 2026-07-19

### Added
- `Example_turnBoundary` and expanded `turn_boundary_test.go` (premature stream.end, stopped, empty content, phase early-complete)

### Changed
- Handshake `ClientVersion` reports `0.4.5`

## [0.4.4] - 2026-07-19

### Changed
- `TurnRunner` always ends turns via `TurnBoundary` (DaemonSession contract: gated `soothe.stream.end` / `status.idle` / `status.stopped`). `EventClassifier` is content + optional phase early-complete only — not the sole turn terminator
- Handshake `ClientVersion` reports `0.4.4`

### Added
- `TurnBoundary` + `IsDaemonTurnEndEvent` for pool-path turn end parity with `DaemonSession.IterTurnChunks`

## [0.4.3] - 2026-07-19

### Added
- `TreatStreamEndAsComplete` — soft-complete on turn-scoped `soothe.stream.end` with accumulated text (DaemonSession / soothe-cli parity)
- `GateTurnEndSignals` — when set with `TreatStatusIdleAsComplete`, ignore pre-running `status=idle` (continue-thread stub race)
- `TurnLifecycleGate` + `ClassifyTurn` — per-turn running/progress gates; `TurnRunner` allocates a gate when stream-end or gated idle is enabled
- `soothe.stream.end` recognized by `IsDeliverableCompletionEvent`

### Changed
- Handshake `ClientVersion` reports `0.4.3`

## [0.4.2] - 2026-07-19

### Removed
- Legacy loop phase `direct_model` from `DefaultDeliverablePhases` and `IsLoopAssistantPhase`
- Unphased `mode=messages` AI text no longer auto-completes a turn (accumulate only; finish via named phases / `status.idle`)
- Reject legacy `intent_hint` values `direct_llm`, `quiz`, and `direct_model`

### Changed
- Handshake `ClientVersion` reports `0.4.2`

## [0.4.1] - 2026-07-19

### Fixed
- `ReceiveMessages` now sends `delivery_ack` on terminal frames (parity with `ReadEvent`), preventing daemon 30s ack-drain stalls (Triarch IG-015)

### Added
- `chitchat` in `DefaultDeliverablePhases` and `IsLoopAssistantPhase` (SOCIAL fast-path)
- `AppKey` type + `ContextWithAppKey` / `AppKeyFromContext` — product conversation key, not a daemon id
- `ConnectionPool` ignores `pending-*` loop ids and preserves `SessionType` on create
- `QueryGate.SetSendCancel` / `ReplaceCancel` for HTTP pre-reserve + turn wiring
- `TurnRunner.ExecuteReserved` and `WithErrorData` for product SSE error payloads

### Changed
- Handshake `ClientVersion` reports `0.4.1`
- Pool/gate/turn/store APIs use `appKey` naming (was overloaded `sessionID`)
- `SessionEntry.AppKey` replaces `SessionEntry.SessionID`
- `BootstrapFunc` takes only daemon scope (`workspaceID`, `userID`); product `AppKey` via context
- `make verify` / CI require `golangci-lint` (errcheck Close handling in tests)


## [0.4.0] - 2026-07-18

### Changed
- Align subagent wire event constants with daemon names: `EventExplorer*` (`soothe.subagent.explorer.*`) and `EventDeepResearch*` (`soothe.subagent.deep_research.*`)
- Handshake `ClientVersion` reports `0.4.0`
- Examples use `preferred_subagent` values `explorer` / `deep_research`

### Removed
- Legacy `EventExplore*` / `EventTacitus*` constants and `soothe.subagent.explore.*` / `soothe.subagent.tacitus.*` namespaces

## [0.3.1] - 2026-07-17

### Added
- Full protocol-1 `autopilot_*` WebSocket RPCs on `Client` and `CommandClient` (status, submit, goals, wake/dream/resume, list/get jobs)

### Changed
- Handshake `ClientVersion` reports `0.3.1`
- README documents WebSocket-only autopilot control (no HTTP REST)

## [0.3.0] - 2026-07-17

### Added
- `appkit.DaemonSession` dual-socket turn streaming (`SendTurn`, `IterTurnChunks`, `EnsureConnected`)
- `CommandClient` for ephemeral jobs/cron one-shot RPCs
- Stream `delivery_ack` on terminal frames for daemon drain gating
- Priority-aware inbound backpressure with `InboundDropped` / `SetStreamDegradedCallback`
- Progressive examples `01`–`06` (hello → jobs)

### Changed
- Job blocking helpers use a single `RequestResponse` (no double-send)
- Connection pool enforces `MaxIdleTime` on acquire
- Handshake `ClientVersion` reports `0.3.0`
- README documents the four API entry-point tiers

### Fixed
- Job lifecycle example expectations aligned with single-RPC create

## [0.2.6] - 2026-07-15

See GitHub release notes for prior history.
