package appkit

import "time"

// SessionEntry is the persisted mapping between an application session id and
// the daemon loop id, plus bookkeeping for resume and reset.
type SessionEntry struct {
	WorkspaceID string
	SessionID   string
	LoopID      string
	SessionType string // app-defined taxonomy (e.g. "primary" | "ephemeral")
	Purpose     string // optional app key for ephemeral internal features
	IsActive    bool
	ResetCount  int
	LastUsedAt  time.Time
}

// SessionMessage is a persisted message row (assistant reply or error) written
// back through the store when a turn completes.
type SessionMessage struct {
	ID       string
	Role     string // "assistant" | "user" | "error"
	Content  string
	Context  interface{}
	Metadata map[string]interface{}
}

// SessionStore is the persistence seam between appkit and the application's
// storage backend (triarch: Postgres; other apps: in-memory, Redis, etc.).
// Implementations must be safe for concurrent use.
//
// appkit's ConnectionPool consults the store to decide whether to bootstrap a
// fresh loop (no loop id on file) or reattach to an existing one, and records
// the loop id once bootstrapped. TurnRunner persists the final assistant reply
// and error rows via AppendMessage.
type SessionStore interface {
	// GetSession returns the persisted entry for sessionID, or
	// (nil, nil) if no record exists.
	GetSession(sessionID string) (*SessionEntry, error)

	// CreateSession persists a new session↔loop mapping.
	CreateSession(workspaceID, sessionID, loopID, sessionType string) error

	// UpdateLastUsed stamps the session's last-used timestamp.
	UpdateLastUsed(sessionID string) error

	// IncrementResetCount bumps the reset counter (used to decide fresh
	// bootstrap vs reattach after an explicit reset).
	IncrementResetCount(sessionID string) error

	// GetLoopIDForSession returns the daemon loop id for sessionID and whether
	// one is on file. ok==false triggers a fresh loop_new bootstrap.
	GetLoopIDForSession(sessionID string) (loopID string, ok bool, err error)

	// AppendMessage writes a message row (assistant reply, error, etc.) for
	// the session. metadata carries optional query analytics.
	AppendMessage(sessionID string, message SessionMessage) error
}
