package appkit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	soothe "github.com/mirasoth/soothe-client-go"
)

func TestUnwrapNext(t *testing.T) {
	inner := map[string]interface{}{"type": "event", "mode": "messages", "data": []interface{}{}}
	frame := map[string]interface{}{
		"type": "next",
		"payload": map[string]interface{}{
			"data": inner,
		},
	}
	got := UnwrapNext(frame)
	if asString(got["type"]) != "event" {
		t.Fatalf("expected event, got %#v", got)
	}
}

func TestShouldDropStreamChunkEarly(t *testing.T) {
	if !ShouldDropStreamChunkEarly(nil, "updates", map[string]interface{}{}) {
		t.Fatal("empty updates should drop")
	}
	if ShouldDropStreamChunkEarly(nil, "custom", map[string]interface{}{"type": "x"}) {
		t.Fatal("custom should not drop")
	}
}

func writeJSON(conn *websocket.Conn, v interface{}) {
	b, _ := json.Marshal(v)
	_ = conn.WriteMessage(websocket.TextMessage, b)
}

func TestDaemonSession_IterTurnChunks_StreamEnd(t *testing.T) {
	upgrader := websocket.Upgrader{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var env map[string]interface{}
			if json.Unmarshal(data, &env) != nil {
				continue
			}
			typ, _ := env["type"].(string)
			method, _ := env["method"].(string)
			id, _ := env["id"].(string)

			if typ == "connection_init" {
				writeJSON(conn, map[string]interface{}{
					"proto": "1", "type": "status", "state": "idle",
				})
				writeJSON(conn, map[string]interface{}{
					"proto": "1", "type": "connection_ack",
					"result": map[string]interface{}{
						"protocol_version":      "1",
						"readiness_state":       "ready",
						"capabilities":          []string{"streaming", "batch", "heartbeat"},
						"heartbeat_interval_ms": 0,
					},
				})
				continue
			}
			if typ == "request" && method == "loop_new" {
				writeJSON(conn, map[string]interface{}{
					"proto": "1", "type": "response", "id": id,
					"result": map[string]interface{}{"loop_id": "loop-test-1"},
				})
				continue
			}
			if typ == "subscribe" && method == "loop_events" {
				writeJSON(conn, map[string]interface{}{
					"proto": "1", "type": "next", "id": id,
					"payload": map[string]interface{}{"success": true},
				})
				continue
			}
			if typ == "notification" && method == "loop_input" {
				subID := "sub-1" // unused; stream frames are unsolicited maps for ReadEvent
				_ = subID
				writeJSON(conn, map[string]interface{}{
					"type": "status", "state": "running", "loop_id": "loop-test-1", "turn_id": "loop-test-1:1",
				})
				writeJSON(conn, map[string]interface{}{
					"type": "event", "mode": "messages", "loop_id": "loop-test-1", "turn_id": "loop-test-1:1",
					"namespace": []interface{}{},
					"data": []interface{}{
						map[string]interface{}{"type": "ai", "content": "hi"},
						map[string]interface{}{},
					},
				})
				writeJSON(conn, map[string]interface{}{
					"type": "event", "mode": "custom", "loop_id": "loop-test-1", "turn_id": "loop-test-1:1",
					"namespace": []interface{}{},
					"data": map[string]interface{}{
						"type": "soothe.stream.end", "scope": "turn", "turn_id": "loop-test-1:1",
					},
				})
				continue
			}
			if typ == "notification" {
				continue
			}
			if typ == "request" {
				writeJSON(conn, map[string]interface{}{
					"proto": "1", "type": "response", "id": id,
					"result": map[string]interface{}{"ok": true},
				})
			}
		}
	}))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")
	cfg := soothe.DefaultConfig()
	cfg.LoopStatusTimeout = 5 * time.Second
	cfg.SubscriptionTimeout = 5 * time.Second
	cfg.DaemonReadyTimeout = 5 * time.Second

	session := NewDaemonSession(wsURL, &DaemonSessionOptions{Config: cfg, PostIdleDrain: 50 * time.Millisecond})
	defer func() { _ = session.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := session.Connect(ctx, ""); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := session.SendTurn(ctx, "hello", nil); err != nil {
		t.Fatalf("send: %v", err)
	}

	chunks, errCh := session.IterTurnChunks(ctx, 5*time.Second)
	var modes []string
	for chunk := range chunks {
		modes = append(modes, chunk.Mode)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("iter: %v", err)
	}
	if len(modes) == 0 {
		t.Fatal("expected at least one chunk")
	}
	if session.LastTurnEndState != "stream_end" {
		t.Fatalf("unexpected end state %q modes=%v", session.LastTurnEndState, modes)
	}
}

// TestDaemonSession_SetClarificationMode verifies the RPC helper sends
// loop_set_clarification_mode with mode + optional interaction_mode and
// returns applied=true from the daemon response.
func TestDaemonSession_SetClarificationMode(t *testing.T) {
	var seenParams map[string]interface{}
	upgrader := websocket.Upgrader{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var env map[string]interface{}
			if json.Unmarshal(data, &env) != nil {
				continue
			}
			typ, _ := env["type"].(string)
			method, _ := env["method"].(string)
			id, _ := env["id"].(string)

			if typ == "connection_init" {
				writeJSON(conn, map[string]interface{}{
					"proto": "1", "type": "status", "state": "idle",
				})
				writeJSON(conn, map[string]interface{}{
					"proto": "1", "type": "connection_ack",
					"result": map[string]interface{}{
						"protocol_version":      "1",
						"readiness_state":       "ready",
						"capabilities":          []string{"streaming", "batch", "heartbeat"},
						"heartbeat_interval_ms": 0,
					},
				})
				continue
			}
			if typ == "request" && method == "loop_new" {
				writeJSON(conn, map[string]interface{}{
					"proto": "1", "type": "response", "id": id,
					"result": map[string]interface{}{"loop_id": "loop-cl-mode-1"},
				})
				continue
			}
			if typ == "subscribe" && method == "loop_events" {
				writeJSON(conn, map[string]interface{}{
					"proto": "1", "type": "next", "id": id,
					"payload": map[string]interface{}{"success": true},
				})
				continue
			}
			if typ == "request" && method == "loop_set_clarification_mode" {
				seenParams = env["params"].(map[string]interface{})
				writeJSON(conn, map[string]interface{}{
					"proto": "1", "type": "response", "id": id,
					"result": map[string]interface{}{"applied": true},
				})
				continue
			}
			if typ == "request" {
				writeJSON(conn, map[string]interface{}{
					"proto": "1", "type": "response", "id": id,
					"result": map[string]interface{}{"ok": true},
				})
			}
		}
	}))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")
	cfg := soothe.DefaultConfig()
	cfg.LoopStatusTimeout = 5 * time.Second
	cfg.SubscriptionTimeout = 5 * time.Second
	cfg.DaemonReadyTimeout = 5 * time.Second

	session := NewDaemonSession(wsURL, &DaemonSessionOptions{Config: cfg})
	defer func() { _ = session.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := session.Connect(ctx, ""); err != nil {
		t.Fatalf("connect: %v", err)
	}

	applied, err := session.SetClarificationMode(ctx, "auto", "bypass")
	if err != nil {
		t.Fatalf("set clarification mode: %v", err)
	}
	if !applied {
		t.Fatal("expected applied=true")
	}
	if seenParams == nil {
		t.Fatal("expected loop_set_clarification_mode request on RPC socket")
	}
	if got := seenParams["mode"]; got != "auto" {
		t.Fatalf("mode param = %v, want auto", got)
	}
	if got := seenParams["interaction_mode"]; got != "bypass" {
		t.Fatalf("interaction_mode param = %v, want bypass", got)
	}
	if got := seenParams["loop_id"]; got != "loop-cl-mode-1" {
		t.Fatalf("loop_id param = %v, want loop-cl-mode-1", got)
	}
}

// TestDaemonSession_SetClarificationMode_NoLoop verifies the helper returns
// applied=false with no RPC when no loop_id is bound.
func TestDaemonSession_SetClarificationMode_NoLoop(t *testing.T) {
	session := NewDaemonSession("ws://invalid.invalid", &DaemonSessionOptions{})
	defer func() { _ = session.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	applied, err := session.SetClarificationMode(ctx, "manual", "")
	if err != nil {
		t.Fatalf("expected nil error with no loop, got: %v", err)
	}
	if applied {
		t.Fatal("expected applied=false when no loop_id is bound")
	}
}

// TestDaemonSession_SetClarificationMode_OmitInteractionMode verifies the
// interaction_mode field is omitted from the RPC payload when interactionMode
// is empty (Python "omit when None" parity).
func TestDaemonSession_SetClarificationMode_OmitInteractionMode(t *testing.T) {
	var seenParams map[string]interface{}
	upgrader := websocket.Upgrader{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var env map[string]interface{}
			if json.Unmarshal(data, &env) != nil {
				continue
			}
			typ, _ := env["type"].(string)
			method, _ := env["method"].(string)
			id, _ := env["id"].(string)

			if typ == "connection_init" {
				writeJSON(conn, map[string]interface{}{
					"proto": "1", "type": "status", "state": "idle",
				})
				writeJSON(conn, map[string]interface{}{
					"proto": "1", "type": "connection_ack",
					"result": map[string]interface{}{
						"protocol_version":      "1",
						"readiness_state":       "ready",
						"capabilities":          []string{"streaming", "batch", "heartbeat"},
						"heartbeat_interval_ms": 0,
					},
				})
				continue
			}
			if typ == "request" && method == "loop_new" {
				writeJSON(conn, map[string]interface{}{
					"proto": "1", "type": "response", "id": id,
					"result": map[string]interface{}{"loop_id": "loop-cl-omit-1"},
				})
				continue
			}
			if typ == "subscribe" && method == "loop_events" {
				writeJSON(conn, map[string]interface{}{
					"proto": "1", "type": "next", "id": id,
					"payload": map[string]interface{}{"success": true},
				})
				continue
			}
			if typ == "request" && method == "loop_set_clarification_mode" {
				seenParams = env["params"].(map[string]interface{})
				writeJSON(conn, map[string]interface{}{
					"proto": "1", "type": "response", "id": id,
					"result": map[string]interface{}{"applied": false},
				})
				continue
			}
			if typ == "request" {
				writeJSON(conn, map[string]interface{}{
					"proto": "1", "type": "response", "id": id,
					"result": map[string]interface{}{"ok": true},
				})
			}
		}
	}))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")
	cfg := soothe.DefaultConfig()
	cfg.LoopStatusTimeout = 5 * time.Second
	cfg.SubscriptionTimeout = 5 * time.Second
	cfg.DaemonReadyTimeout = 5 * time.Second

	session := NewDaemonSession(wsURL, &DaemonSessionOptions{Config: cfg})
	defer func() { _ = session.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := session.Connect(ctx, ""); err != nil {
		t.Fatalf("connect: %v", err)
	}

	applied, err := session.SetClarificationMode(ctx, "manual", "")
	if err != nil {
		t.Fatalf("set clarification mode: %v", err)
	}
	if applied {
		t.Fatal("expected applied=false from daemon")
	}
	if seenParams == nil {
		t.Fatal("expected loop_set_clarification_mode request on RPC socket")
	}
	if _, exists := seenParams["interaction_mode"]; exists {
		t.Fatal("interaction_mode should be omitted when empty")
	}
	if got := seenParams["mode"]; got != "manual" {
		t.Fatalf("mode param = %v, want manual", got)
	}
}
