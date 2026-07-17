# Changelog

All notable changes to `soothe-client-go` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
