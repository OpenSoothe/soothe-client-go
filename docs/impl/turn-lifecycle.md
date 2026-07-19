# Appkit turn lifecycle features

Notes for `appkit` idle timeout, status-idle completion, attachment compaction,
subscription-metadata filtering, and soft stream-close policies.

## Behaviour summary

| Knob | Default | Notes |
|------|---------|--------|
| `IdleTimeout` | off (`0`) | Silence watchdog between events |
| `MinIdleTimeoutWithAttachments` | off | Raises idle when attachments are present |
| `TreatStatusIdleAsComplete` | `false` | Opt-in: `status=idle` + content ends the turn |
| `TreatStreamEndAsComplete` | `false` | Opt-in: turn-scoped `soothe.stream.end` + content ends the turn (always gated) |
| `GateTurnEndSignals` | `false` | With idle opt-in: require `TurnLifecycleGate` running + payload (DaemonSession) |
| `CompactAttachmentsBeforeSend` | `false` | Opt-in image downscale before `loop_input` |
| `OnIdleTimeout` / `OnQueryTimeout` / `OnStreamClose` | fail | Soft-complete optional via `TimeoutPolicySoftComplete` |
| `plan_direct` in `DefaultDeliverablePhases` | excluded | Streamable narration only; not a turn terminal |

`TurnRunner` creates a per-turn `TurnLifecycleGate` when `TreatStreamEndAsComplete` or
`GateTurnEndSignals` is set, and classifies via `ClassifyTurn` so concurrent chats
sharing one classifier do not race on gate state.

## Tests

See `appkit/turn_lifecycle_test.go` and `intent_hints_test.go`.
