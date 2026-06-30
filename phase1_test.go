package soothe

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// testConcurrentRPCHandler handshakes, then for each `request` it receives it
// records the (method, id) and delays the response so responses can be sent
// out of order. Methods "slow_a" and "slow_b" are answered in reverse arrival
// order to exercise the multiplexer's (type, id) routing.
func testConcurrentRPCHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	var writeMu sync.Mutex // serializes server-side writes (gorilla forbids concurrent writes)

	send := func(id string, result map[string]interface{}) {
		writeMu.Lock()
		defer writeMu.Unlock()
		testSendResponse(conn, id, result)
	}

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var m map[string]interface{}
		if err := json.Unmarshal(msg, &m); err != nil {
			continue
		}
		if isConnectionInit(m) {
			writeMu.Lock()
			testSendHandshake(conn, m)
			writeMu.Unlock()
			continue
		}
		typ, _ := m["type"].(string)
		if typ != "request" {
			continue
		}
		id, _ := m["id"].(string)
		method, _ := m["method"].(string)

		// For slow_a/slow_b: spawn a goroutine that waits then responds, so
		// responses can be sent out of order. Writes are serialized via writeMu.
		if method == "slow_a" || method == "slow_b" {
			go func(id, method string) {
				delay := 50 * time.Millisecond
				if method == "slow_a" {
					delay = 300 * time.Millisecond // slow_a arrives first, responds last
				}
				time.Sleep(delay)
				send(id, map[string]interface{}{"method": method})
			}(id, method)
			continue
		}
		// Default: echo immediately.
		send(id, map[string]interface{}{"method": method})
	}
}

// TestClient_RequestResponse_ConcurrentOutOfOrder is the multiplexer regression
// test (IG-527): two concurrent RequestResponse calls with out-of-order
// responses must both resolve to the correct caller. Without the (type, id)
// multiplexer, the second caller would discard the first caller's response.
func TestClient_RequestResponse_ConcurrentOutOfOrder(t *testing.T) {
	ts := newTestServer(testConcurrentRPCHandler)
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	// Start a ReceiveMessages reader so the multiplexer's concurrent path is
	// exercised (the reader routes frames to waiters).
	rctx, rcancel := context.WithCancel(context.Background())
	defer rcancel()
	if _, err := client.ReceiveMessages(rctx); err != nil {
		t.Fatalf("ReceiveMessages: %v", err)
	}

	type result struct {
		method string
		err    error
	}
	results := make(chan result, 2)

	go func() {
		resp, err := client.RequestResponse(ctx, map[string]interface{}{"type": "slow_a"}, "slow_a", 5*time.Second)
		if err != nil {
			results <- result{err: err}
			return
		}
		results <- result{method: resp["method"].(string)}
	}()
	go func() {
		resp, err := client.RequestResponse(ctx, map[string]interface{}{"type": "slow_b"}, "slow_b", 5*time.Second)
		if err != nil {
			results <- result{err: err}
			return
		}
		results <- result{method: resp["method"].(string)}
	}()

	for i := 0; i < 2; i++ {
		select {
		case r := <-results:
			if r.err != nil {
				t.Errorf("RPC failed: %v", r.err)
				continue
			}
			if r.method != "slow_a" && r.method != "slow_b" {
				t.Errorf("unexpected method: %v", r.method)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for concurrent RPC results")
		}
	}
}

// TestClient_Disconnected_Unclean verifies a read failure fires an unclean drop.
func TestClient_Disconnected_Unclean(t *testing.T) {
	// Handler handshakes then closes immediately, simulating an abrupt drop.
	handler := func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var m map[string]interface{}
			if json.Unmarshal(msg, &m) != nil {
				continue
			}
			if isConnectionInit(m) {
				testSendHandshake(conn, m)
				// Abrupt close (no close frame).
				conn.Close()
				return
			}
		}
	}
	ts := newTestServer(handler)
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	rctx, rcancel := context.WithCancel(context.Background())
	defer rcancel()
	if _, err := client.ReceiveMessages(rctx); err != nil {
		t.Fatalf("ReceiveMessages: %v", err)
	}

	select {
	case cause := <-client.Disconnected():
		if cause != DisconnectUnclean {
			t.Errorf("expected DisconnectUnclean, got %v", cause)
		}
	case <-time.After(2 * time.Second):
		t.Error("expected Disconnected() to fire on abrupt drop")
	}
}

// testReattachHandler handshakes; loop_reattach returns success, loop_subscribe
// confirms, and loop_get either succeeds (live) or returns LOOP_NOT_FOUND
// (-32200) (stale), controlled by the stale flag set via the request params.
func testReattachHandler(stale bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var m map[string]interface{}
			if json.Unmarshal(msg, &m) != nil {
				continue
			}
			if isConnectionInit(m) {
				testSendHandshake(conn, m)
				continue
			}
			typ, _ := m["type"].(string)
			method, _ := m["method"].(string)
			id, _ := m["id"].(string)
			switch {
			case typ == "request" && method == "loop_reattach":
				testSendResponse(conn, id, map[string]interface{}{"loop_id": "loop-x", "status": "reattached"})
			case typ == "subscribe" && method == "loop_events":
				// subscribe confirmation: next frame with success=true
				env := map[string]interface{}{"proto": "1", "type": "next", "id": id, "payload": map[string]interface{}{"loop_id": "loop-x", "success": true}}
				b, _ := json.Marshal(env)
				conn.WriteMessage(websocket.TextMessage, b)
			case typ == "request" && method == "loop_get":
				if stale {
					testSendError(conn, id, -32200, "loop not found")
				} else {
					testSendResponse(conn, id, map[string]interface{}{"loop_id": "loop-x", "state": "idle"})
				}
			case typ == "request":
				testSendResponse(conn, id, map[string]interface{}{"echoed": method})
			}
		}
	}
}

func TestClient_ReattachAndProbe_Live(t *testing.T) {
	ts := newTestServer(testReattachHandler(false))
	defer ts.Close()
	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	if err := client.ReattachAndProbe(ctx, "loop-x"); err != nil {
		t.Errorf("ReattachAndProbe (live) failed: %v", err)
	}
}

func TestClient_ReattachAndProbe_Stale(t *testing.T) {
	ts := newTestServer(testReattachHandler(true))
	defer ts.Close()
	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	err := client.ReattachAndProbe(ctx, "loop-x")
	if err == nil {
		t.Fatal("expected StaleLoopError, got nil")
	}
	var sle *StaleLoopError
	if !errors.As(err, &sle) {
		t.Errorf("expected *StaleLoopError, got %T: %v", err, err)
	}
	if sle.LoopID != "loop-x" {
		t.Errorf("expected LoopID loop-x, got %s", sle.LoopID)
	}
}

// testWarnThenReadyHandler handshakes with a Warn error (DAEMON_STARTING
// -32001) on the first connection_init, then succeeds with readiness "ready"
// on the retry. Verifies the handshake retries on Warn-severity codes.
func testWarnThenReadyHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	sentWarn := false
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var m map[string]interface{}
		if json.Unmarshal(msg, &m) != nil {
			continue
		}
		if !isConnectionInit(m) {
			continue
		}
		if !sentWarn {
			// First init: emit a Warn-severity error, then a status, then the ack.
			conn.WriteMessage(websocket.TextMessage, []byte(`{"proto":"1","type":"status","state":"idle"}`))
			errEnv, _ := json.Marshal(map[string]interface{}{
				"proto": "1", "type": "error", "id": m["id"],
				"error": map[string]interface{}{"code": -32001, "message": "daemon starting"},
			})
			conn.WriteMessage(websocket.TextMessage, errEnv)
			sentWarn = true
			continue
		}
		// Retry init: succeed.
		testSendHandshake(conn, m)
	}
}

func TestClient_Handshake_RetryOnWarnCode(t *testing.T) {
	ts := newTestServer(testWarnThenReadyHandler)
	defer ts.Close()
	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect should succeed after Warn retry: %v", err)
	}
	defer client.Close()
	if !client.IsHandshakeComplete() {
		t.Error("handshake should be complete after Warn retry")
	}
}

// testReconnectHandler is a stateful handler that survives a single reconnect:
// it tracks connection count and handshakes normally each time.
func testReconnectHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var m map[string]interface{}
		if json.Unmarshal(msg, &m) != nil {
			continue
		}
		if isConnectionInit(m) {
			testSendHandshake(conn, m)
			continue
		}
		typ, _ := m["type"].(string)
		method, _ := m["method"].(string)
		id, _ := m["id"].(string)
		if typ == "request" {
			testSendResponse(conn, id, map[string]interface{}{"echoed": method})
		}
	}
}

func TestClient_Reconnect(t *testing.T) {
	ts := newTestServer(testReconnectHandler)
	defer ts.Close()
	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}

	// Simulate a drop by closing the underlying socket, then reconnect.
	if err := client.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Disconnected() should have fired (unclean, from Close).
	select {
	case <-client.Disconnected():
	case <-time.After(time.Second):
		t.Error("expected Disconnected() after Close")
	}

	if err := client.Reconnect(ctx); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	defer client.Close()
	if !client.IsConnected() {
		t.Error("should be connected after reconnect")
	}
	// Verify RPC works on the reconnected socket.
	resp, err := client.RequestResponse(ctx, map[string]interface{}{"type": "ping"}, "ping", 5*time.Second)
	if err != nil {
		t.Fatalf("RPC after reconnect: %v", err)
	}
	if resp["echoed"] != "ping" {
		t.Errorf("unexpected response: %v", resp)
	}
}
