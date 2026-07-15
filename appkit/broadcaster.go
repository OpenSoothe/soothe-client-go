package appkit

import (
	"sync"

	"github.com/google/uuid"
)

// SSEEvent is one Server-Sent Event payload. The Type vocabulary
// ("delta"/"complete"/"query_error"/"status_change"/...) is app-defined.
type SSEEvent struct {
	Type string
	Data interface{}
}

// SSEBroadcaster is a generic, string-keyed pub/sub fan-out for SSE-style
// event delivery. Applications map their own session ids at the boundary.
// Non-blocking sends drop on a full subscriber channel so a slow consumer
// cannot stall the broadcaster.
type SSEBroadcaster struct {
	subscribers map[string]map[string]chan SSEEvent
	mu          sync.RWMutex
}

// NewSSEBroadcaster creates an empty broadcaster.
func NewSSEBroadcaster() *SSEBroadcaster {
	return &SSEBroadcaster{
		subscribers: make(map[string]map[string]chan SSEEvent),
	}
}

// Subscribe registers a new subscriber channel for a session id. The returned
// channel is buffered (cap 100) so a transient slow consumer does not drop
// events immediately.
func (b *SSEBroadcaster) Subscribe(sessionID string) (<-chan SSEEvent, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan SSEEvent, 100)
	subscriberID := uuid.New().String()

	if b.subscribers[sessionID] == nil {
		b.subscribers[sessionID] = make(map[string]chan SSEEvent)
	}
	b.subscribers[sessionID][subscriberID] = ch
	return ch, nil
}

// Unsubscribe removes a subscriber channel (matched by identity) and closes it.
// Safe to call with a nil/unknown channel.
func (b *SSEBroadcaster) Unsubscribe(sessionID string, ch <-chan SSEEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs := b.subscribers[sessionID]
	if subs == nil {
		return
	}
	for id, subCh := range subs {
		// Compare the read-only channels by identity via the underlying value.
		if subCh == ch {
			close(subCh)
			delete(subs, id)
			break
		}
	}
	if len(subs) == 0 {
		delete(b.subscribers, sessionID)
	}
}

// Broadcast sends an event to all subscribers for a session id. Non-blocking:
// a full subscriber channel is skipped (drop-on-full) so one slow consumer
// cannot block the others.
func (b *SSEBroadcaster) Broadcast(sessionID string, event SSEEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	subs := b.subscribers[sessionID]
	if subs == nil {
		return
	}
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
		}
	}
}

// Close closes all subscriber channels for a session id and removes the entry.
func (b *SSEBroadcaster) Close(sessionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs := b.subscribers[sessionID]
	if subs == nil {
		return
	}
	for _, ch := range subs {
		close(ch)
	}
	delete(b.subscribers, sessionID)
}

// CloseAll closes every subscriber channel across all sessions.
func (b *SSEBroadcaster) CloseAll() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for sid, subs := range b.subscribers {
		for _, ch := range subs {
			close(ch)
		}
		delete(b.subscribers, sid)
	}
}
