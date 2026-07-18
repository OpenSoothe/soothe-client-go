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
					"type": "status", "state": "running", "loop_id": "loop-test-1",
				})
				writeJSON(conn, map[string]interface{}{
					"type": "event", "mode": "messages", "loop_id": "loop-test-1",
					"namespace": []interface{}{},
					"data": []interface{}{
						map[string]interface{}{"type": "ai", "content": "hi"},
						map[string]interface{}{},
					},
				})
				writeJSON(conn, map[string]interface{}{
					"type": "event", "mode": "custom", "loop_id": "loop-test-1",
					"namespace": []interface{}{},
					"data": map[string]interface{}{
						"type": "soothe.stream.end", "scope": "turn",
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
