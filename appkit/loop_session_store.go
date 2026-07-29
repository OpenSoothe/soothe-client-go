package appkit

import (
	"context"
	"time"
)

// LoopSessionEntry is the persisted mapping between an application AppKey and
// the daemon loop id, plus bookkeeping for resume and reset.
type LoopSessionEntry struct {
	WorkspaceID string
	AppKey      string // application conversation key (not a daemon id)
	LoopID      string
	SessionType string // app-defined taxonomy (e.g. "primary" | "ephemeral")
	Purpose     string // optional product purpose for ephemeral features
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

// LoopSessionStore is the persistence seam between appkit and the application's
// storage backend (in-memory, Postgres, Redis, etc.). Implementations must be
// safe for concurrent use.
//
// Keys are AppKey values (product conversation ids). appkit maps AppKey →
// daemon loop_id; the daemon never sees AppKey on the wire.
type LoopSessionStore interface {
	// GetSession returns the persisted entry for appKey, or (nil, nil) if none.
	GetSession(ctx context.Context, appKey AppKey) (*LoopSessionEntry, error)

	// CreateSession persists a new AppKey↔loop mapping.
	CreateSession(ctx context.Context, workspaceID string, appKey AppKey, loopID, sessionType string) error

	// UpdateLastUsed stamps the entry's last-used timestamp.
	UpdateLastUsed(ctx context.Context, appKey AppKey) error

	// IncrementResetCount bumps the reset counter (fresh bootstrap vs reattach).
	IncrementResetCount(ctx context.Context, appKey AppKey) error

	// GetLoopIDForSession returns the daemon loop id for appKey.
	// ok==false triggers a fresh loop_new bootstrap.
	GetLoopIDForSession(ctx context.Context, appKey AppKey) (loopID string, ok bool, err error)

	// AppendMessage writes a message row for the AppKey's conversation.
	AppendMessage(ctx context.Context, appKey AppKey, message SessionMessage) error
}
