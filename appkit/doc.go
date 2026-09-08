// Package appkit is the application layer over the core Client:
// dual-socket DaemonSession, per-AppKey connection pooling, single-flight
// query gating, turn execution, event→deliverable classification, and
// SSE-style fan-out.
//
// AppKey is a product conversation key (not a daemon loop_id or client_id).
// appkit is product-agnostic: deliverable phases, persistence, and SSE event
// vocabulary are supplied via configuration and LoopSessionStore.
//
// API tiers:
//   - One conversation, streamed turns → DaemonSession
//   - Multi-user HTTP backend → ConnectionPool + TurnRunner
//   - Jobs / cron one-shots → soothe.CommandClient (core package)
//
// Recommended cancel ordering on teardown: QueryGate.Cancel(appKey) → wait
// for in-flight Execute → ConnectionPool.Release / Stop → Client.Close.
package appkit
