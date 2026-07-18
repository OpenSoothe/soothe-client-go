// Package appkit is the application-architecture layer over the core Client:
// dual-socket DaemonSession, per-AppKey connection pooling, single-flight
// query gating, turn execution, event→deliverable classification, and
// SSE-style fan-out.
//
// AppKey is a product conversation key (not a daemon loop_id or client_id).
// appkit is product-agnostic: deliverable phases, persistence, and SSE event
// vocabulary are supplied via configuration and SessionStore.
//
// # Stack
//
//   - Core soothe.Client — transport, handshake, multiplexed RPC + event stream.
//   - This package — DaemonSession, ConnectionPool, QueryGate, TurnRunner,
//     EventClassifier, SSEBroadcaster, SessionStore.
//   - The application — domain types, persistence implementation, product
//     config, user-facing copy.
//
// # API tiers
//
//   - One conversation, streamed turns → DaemonSession
//   - Multi-user HTTP backend → ConnectionPool + TurnRunner
//   - Jobs / cron one-shots → soothe.CommandClient (core package)
//
// Typical wiring for backends: construct a ConnectionPool with the daemon
// WebSocket URL and a SessionStore, then build a TurnRunner from the pool,
// QueryGate, EventClassifier, SessionStore, and SSEBroadcaster.
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
// Recommended cancel ordering: QueryGate.Cancel(appKey) → wait for
// in-flight Execute (caller-owned WaitGroup) → ConnectionPool.Release / Stop →
// Client.Close. Do not Close the WebSocket under an active reader without
// cancelling the turn first.
package appkit
