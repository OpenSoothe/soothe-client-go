package appkit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrQueryBusy is returned when an AppKey already has an in-flight query.
var ErrQueryBusy = errors.New("appkit: query already in progress for app key")

// QueryGate enforces single-flight query execution per AppKey and
// cancel-before-context ordering: when a query is cancelled, the daemon is
// told to stop (command_request{command:"cancel"}) BEFORE the local context
// is cancelled, on a detached timeout so the caller's cancellation cannot
// block the wire send.
type QueryGate struct {
	mu          sync.Mutex
	cancels     map[string]context.CancelFunc          // appKey → query cancel
	sendCancels map[string]func(context.Context) error // appKey → daemon-cancel sender
}

// NewQueryGate constructs an empty gate.
func NewQueryGate() *QueryGate {
	return &QueryGate{
		cancels:     make(map[string]context.CancelFunc),
		sendCancels: make(map[string]func(context.Context) error),
	}
}

// Acquire reserves appKey for one agent turn. It returns ErrQueryBusy if a
// query is already in flight. The caller must pass the CancelFunc for the
// query's timeout context (the same one TurnRunner will derive from).
// sendCancel is the daemon-cancel sender (sends command_request{cancel} for
// the loop); it is invoked from Cancel on a detached 10s context.
func (g *QueryGate) Acquire(appKey string, cancel context.CancelFunc, sendCancel func(context.Context) error) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.cancels[appKey] != nil {
		return ErrQueryBusy
	}
	g.cancels[appKey] = cancel
	if sendCancel != nil {
		g.sendCancels[appKey] = sendCancel
	}
	return nil
}

// Cancel cooperatively stops a running query for appKey. It sends the
// daemon cancel (on a detached 10s-timeout context so caller cancellation
// cannot block the wire send) BEFORE cancelling the local context. Returns
// nil if no query is in flight (intent already satisfied).
func (g *QueryGate) Cancel(appKey string) error {
	g.mu.Lock()
	cancel := g.cancels[appKey]
	sendCancel := g.sendCancels[appKey]
	delete(g.cancels, appKey)
	delete(g.sendCancels, appKey)
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
			_ = fmt.Errorf("appkit: daemon cancel failed for %s: %w", appKey, err)
		}
	}

	if cancel != nil {
		cancel()
	}
	return nil
}

// Release clears the gate for appKey without sending a daemon cancel. Call
// when a query completes normally (success or local failure) so the next turn
// can acquire.
func (g *QueryGate) Release(appKey string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.cancels, appKey)
	delete(g.sendCancels, appKey)
}

// SetSendCancel updates the daemon-cancel sender for an already-acquired
// session (e.g. HTTP handler reserved the gate before the turn knew loopID).
func (g *QueryGate) SetSendCancel(appKey string, sendCancel func(context.Context) error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.cancels[appKey] == nil {
		return
	}
	if sendCancel != nil {
		g.sendCancels[appKey] = sendCancel
	}
}

// ReplaceCancel updates the local CancelFunc for an already-acquired session
// (e.g. turn replaces the handler cancel with a timeout-derived cancel).
func (g *QueryGate) ReplaceCancel(appKey string, cancel context.CancelFunc) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.cancels[appKey] == nil || cancel == nil {
		return
	}
	g.cancels[appKey] = cancel
}

// IsActive reports whether a query is in flight for appKey.
func (g *QueryGate) IsActive(appKey string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.cancels[appKey] != nil
}
