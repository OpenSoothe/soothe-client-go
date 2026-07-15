// Package appkit is the application-architecture layer over the core Client:
// per-session connection pooling, single-flight query gating, turn execution,
// event→deliverable classification, and SSE-style fan-out.
//
// appkit is product-agnostic. Which event phases count as user-facing
// deliverables, how sessions are persisted, and what event type strings the
// frontend expects are supplied by the application via configuration
// (e.g. DeliverablePhases) and interfaces (e.g. SessionStore).
//
// # Layers
//
//   - Core soothe.Client — transport, handshake, multiplexed RPC + event stream.
//   - This package — ConnectionPool, QueryGate, TurnRunner, EventClassifier,
//     SSEBroadcaster, SessionStore.
//   - The application — domain types, persistence implementation, product
//     config, user-facing copy.
//
// Typical wiring: construct a ConnectionPool with the daemon WebSocket URL and
// a SessionStore, then build a TurnRunner from the pool, QueryGate,
// EventClassifier, SessionStore, and SSEBroadcaster, and call Execute per turn.
//
// # Turn timeouts and stream close
//
// TurnConfig supports IdleTimeout (silence watchdog),
// MinIdleTimeoutWithAttachments, soft-complete vs fail policies for idle /
// query / stream-close, and optional attachment compaction. Defaults fail the
// turn on timeout and on event-stream close.
//
// # Session teardown
//
// Recommended cancel ordering: QueryGate.Cancel(sessionID) → wait for
// in-flight Execute (caller-owned WaitGroup) → ConnectionPool.Release / Stop →
// Client.Close. Do not Close the WebSocket under an active reader without
// cancelling the turn first.
package appkit
