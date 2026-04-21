// Package soothe provides a Go client for the Soothe daemon WebSocket API.
//
// The client implements the same protocol as the Python soothe-sdk, providing
// full access to the Soothe daemon's capabilities including thread management,
// event streaming, skills/models discovery, and daemon control.
//
// This package uses a flat structure with all functionality in the root soothe package:
// - Client types and connection management
// - Protocol message types and encoding/decoding
// - Configuration and error types
// - Event constants and verbosity classification
// - Bootstrap and RPC helpers
//
// All types are accessible directly without subpackage imports:
// Config, Client, EventPlanCreated, ConnectionError, VerbosityLevel, etc.
//
// Package: https://github.com/OpenSoothe/soothe-client-go
package soothe
