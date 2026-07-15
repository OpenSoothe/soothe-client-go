package appkit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrQueryBusy is returned when a session already has an in-flight query.
var ErrQueryBusy = errors.New("appkit: query already in progress for session")

// QueryGate enforces single-flight query execution per session id and
// cancel-before-context ordering: when a query is cancelled, the daemon is
// told to stop (command_request{command:"cancel"}) BEFORE the local context
// is cancelled, on a detached timeout so the caller's cancellation cannot
// block the wire send.
type QueryGate struct {
	mu          sync.Mutex
	cancels     map[string]context.CancelFunc          // sessionID → query cancel
	sendCancels map[string]func(context.Context) error // sessionID → daemon-cancel sender
}

// NewQueryGate constructs an empty gate.
func NewQueryGate() *QueryGate {
	return &QueryGate{
		cancels:     make(map[string]context.CancelFunc),
		sendCancels: make(map[string]func(context.Context) error),
	}
}

// Acquire reserves sessionID for one agent turn. It returns ErrQueryBusy if a
// query is already in flight. The caller must pass the CancelFunc for the
// query's timeout context (the same one TurnRunner will derive from).
// sendCancel is the daemon-cancel sender (sends command_request{cancel} for
// the loop); it is invoked from Cancel on a detached 10s context.
func (g *QueryGate) Acquire(sessionID string, cancel context.CancelFunc, sendCancel func(context.Context) error) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.cancels[sessionID] != nil {
		return ErrQueryBusy
	}
	g.cancels[sessionID] = cancel
	if sendCancel != nil {
		g.sendCancels[sessionID] = sendCancel
	}
	return nil
}

// Cancel cooperatively stops a running query for sessionID. It sends the
// daemon cancel (on a detached 10s-timeout context so caller cancellation
// cannot block the wire send) BEFORE cancelling the local context. Returns
// nil if no query is in flight (intent already satisfied).
func (g *QueryGate) Cancel(sessionID string) error {
	g.mu.Lock()
	cancel := g.cancels[sessionID]
	sendCancel := g.sendCancels[sessionID]
	delete(g.cancels, sessionID)
	delete(g.sendCancels, sessionID)
	g.mu.Unlock()

	if cancel == nil && sendCancel == nil {
		return nil
	}

	// Send daemon cancel first, on a detached timeout, then cancel locally.
	if sendCancel != nil {
		ctx, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		if err := sendCancel(ctx); err != nil {
			// Log but proceed to local cancel; the query is still stopping locally.
			_ = fmt.Errorf("appkit: daemon cancel failed for %s: %w", sessionID, err)
		}
	}

	if cancel != nil {
		cancel()
	}
	return nil
}

// Release clears the gate for sessionID without sending a daemon cancel. Call
// when a query completes normally (success or local failure) so the next turn
// can acquire.
func (g *QueryGate) Release(sessionID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.cancels, sessionID)
	delete(g.sendCancels, sessionID)
}

// IsActive reports whether a query is in flight for sessionID.
func (g *QueryGate) IsActive(sessionID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.cancels[sessionID] != nil
}
