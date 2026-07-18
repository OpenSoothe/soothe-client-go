# Changelog

All notable changes to `soothe-client-go` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
