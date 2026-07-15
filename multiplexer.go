package soothe

import (
	"sync"
)

// multiplexer routes inbound protocol-1 frames to the correct waiter by
// (type, id) instead of discarding non-matching events. Concurrent RPCs and
// subscriptions share one read loop; each waiter is keyed by envelope id.
//
// Routing rules:
//   - response/error with id in pending RPCs        → pendingCall (RPC waiter)
//   - next/complete with id in pending subscriptions → pendingSubscription (stream waiter)
//   - everything else                                → not consumed (flows on ReceiveMessages)
//
// ping/pong/disconnect/connection_ack are handled by isControlFrame before
// reaching the multiplexer. receipt_response is keyed by the receipt field;
// support is provided via pendingReceipts.
//
// The multiplexer is consulted by both the ReceiveMessages read loop and the
// synchronous ReadEvent path. A frame routed to a waiter is consumed (returns
// true) and must not be forwarded to the application event channel.
type mux struct {
	mu       sync.Mutex
	rpcs     map[string]*pendingCall
	subs     map[string]*pendingSubscription
	receipts map[string]chan<- map[string]interface{}
}

// pendingCall is a single in-flight RPC wait.
type pendingCall struct {
	id      string
	replyCh chan map[string]interface{} // result on response; nil on error
	errCh   chan error                  // daemon error frame
}

// pendingSubscription is a single in-flight subscription stream.
type pendingSubscription struct {
	id       string
	streamCh chan interface{} // next/complete frames
	done     chan struct{}
}

func newMux() *mux {
	return &mux{
		rpcs:     make(map[string]*pendingCall),
		subs:     make(map[string]*pendingSubscription),
		receipts: make(map[string]chan<- map[string]interface{}),
	}
}

// registerRPC installs a pending RPC wait keyed by id. Returns the pendingCall
// whose replyCh/errCh the caller selects on, and an unregister func that must
// be called when the wait ends (success, timeout, or cancel) to avoid leaks.
func (m *mux) registerRPC(id string) (*pendingCall, func()) {
	pc := &pendingCall{
		id:      id,
		replyCh: make(chan map[string]interface{}, 1),
		errCh:   make(chan error, 1),
	}
	m.mu.Lock()
	m.rpcs[id] = pc
	m.mu.Unlock()
	return pc, func() {
		m.mu.Lock()
		if cur, ok := m.rpcs[id]; ok && cur == pc {
			delete(m.rpcs, id)
		}
		m.mu.Unlock()
	}
}

// registerSubscription installs a pending subscription stream keyed by id.
// Returns the stream channel and an unregister func.
func (m *mux) registerSubscription(id string) (<-chan interface{}, func()) {
	ps := &pendingSubscription{
		id:       id,
		streamCh: make(chan interface{}, 16),
		done:     make(chan struct{}),
	}
	m.mu.Lock()
	m.subs[id] = ps
	m.mu.Unlock()
	return ps.streamCh, func() {
		m.mu.Lock()
		if cur, ok := m.subs[id]; ok && cur == ps {
			delete(m.subs, id)
		}
		close(ps.done)
		m.mu.Unlock()
	}
}

// route inspects one decoded frame (as a map[string]interface{}), delivers it
// to a matching waiter if one exists, and returns consumed=true. Returns
// consumed=false for frames with no matching waiter — these flow on to the
// application event channel. Safe to call from the read goroutine.
func (m *mux) route(frame map[string]interface{}) bool {
	if frame == nil {
		return false
	}
	typ, _ := frame["type"].(string)
	id, _ := frame["id"].(string)

	switch typ {
	case "response", "error":
		if id == "" {
			return false
		}
		m.mu.Lock()
		pc, ok := m.rpcs[id]
		m.mu.Unlock()
		if !ok {
			return false
		}
		if typ == "error" {
			errObj, _ := frame["error"].(map[string]interface{})
			code := -32603
			if ic, ok := errObj["code"].(float64); ok {
				code = int(ic)
			}
			msg, _ := errObj["message"].(string)
			data, _ := errObj["data"].(map[string]interface{})
			select {
			case pc.errCh <- &DaemonError{Code: code, Message: msg, Data: data}:
			default:
			}
			return true
		}
		// response
		result, _ := frame["result"].(map[string]interface{})
		if result == nil {
			result = frame
		}
		select {
		case pc.replyCh <- result:
		default:
		}
		return true

	case "next", "complete":
		if id == "" {
			return false
		}
		m.mu.Lock()
		ps, ok := m.subs[id]
		m.mu.Unlock()
		if !ok {
			return false
		}
		select {
		case <-ps.done:
			return true // waiter gone; consume to avoid re-forwarding
		default:
		}
		select {
		case ps.streamCh <- frame:
		default:
		}
		return true

	case "receipt_response":
		rid, _ := frame["receipt"].(string)
		if rid == "" {
			return false
		}
		m.mu.Lock()
		ch, ok := m.receipts[rid]
		m.mu.Unlock()
		if !ok {
			return false
		}
		select {
		case ch <- frame:
		default:
		}
		return true
	}

	return false
}
