// Package appkit provides the reusable application-architecture layer over a
// soothe-client-go core Client: per-session connection pooling, single-flight
// query gating, turn execution, event→deliverable classification, and SSE
// fan-out (RFC-629).
//
// appkit is product-agnostic. Product decisions — which event phases count as
// user-facing deliverables, how sessions are persisted, what SSE event
// vocabulary the frontend expects — are supplied by the application via
// configuration (e.g. DeliverablePhases) and interfaces (e.g. SessionStore).
//
// # Layers (RFC-629)
//
//   - Layer 0 — the core Client (transport/lifecycle, concurrent multiplexing).
//   - Layer 1 — this package: ConnectionPool, QueryGate, TurnRunner,
//     EventClassifier, SSEBroadcaster, SessionStore.
//   - Layer 2 — the application: domain types, persistence implementation,
//     product config, user-facing copy.
//
// Applications construct a TurnRunner from a ConnectionPool, QueryGate,
// EventClassifier, SessionStore, and SSEBroadcaster, then call Execute per
// query turn. See the IG-527 implementation guide for the end-to-end flow.
package appkit
