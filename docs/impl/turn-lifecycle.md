# Appkit turn lifecycle features

Notes for `appkit` idle timeout, DaemonSession turn boundaries, attachment
compaction, subscription-metadata filtering, and soft stream-close policies.

## Ownership

| Concern | Owner |
|---------|--------|
| **When the turn is over** | `TurnBoundary` (same rules as `DaemonSession.IterTurnChunks`) |
| **What text to stream / persist** | `EventClassifier` (phases, deltas, thinking steps) |
| **Early UX complete** | Optional phase deliverable from classifier (`goal_completion`, `chitchat`, …) before boundary |

`TurnRunner` always feeds `TurnBoundary`. Classifier flags
`TreatStreamEndAsComplete` / `TreatStatusIdleAsComplete` are for standalone
`ClassifyTurn` only — not required for `TurnRunner`.

## Behaviour summary

| Knob | Default | Notes |
|------|---------|--------|
| `IdleTimeout` | off (`0`) | Silence watchdog between events |
| `MinIdleTimeoutWithAttachments` | off | Raises idle when attachments are present |
| `TurnBoundary` (TurnRunner) | always on | Gated `stream.end` / `idle` / `stopped` |
| `TreatStatusIdleAsComplete` | `false` | Standalone ClassifyTurn only |
| `TreatStreamEndAsComplete` | `false` | Standalone ClassifyTurn only |
| `GateTurnEndSignals` | `false` | Standalone ClassifyTurn only |
| `CompactAttachmentsBeforeSend` | `false` | Opt-in image downscale before `loop_input` |
| `OnIdleTimeout` / `OnQueryTimeout` / `OnStreamClose` | fail | Soft-complete optional via `TimeoutPolicySoftComplete` |
| `plan_direct` in `DefaultDeliverablePhases` | excluded | Streamable narration only; not a turn terminal |

## Tests

See `appkit/turn_boundary_test.go`, `appkit/turn_lifecycle_test.go`, and
`examples/appkit` (`Example_turnBoundary`).
