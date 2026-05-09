package soothe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestBootstrapLoopSession_newLoop(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			if err := json.Unmarshal(msg, &m); err != nil {
				continue
			}
			typ, _ := m["type"].(string)

			switch typ {
			case "daemon_ready":
				conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"daemon_ready","state":"ready"}`))
			case "loop_new":
				rid, _ := m["request_id"].(string)
				out := fmt.Sprintf(
					`{"type":"loop_new_response","request_id":%q,"loop_id":"test-loop-456"}`,
					rid,
				)
				conn.WriteMessage(websocket.TextMessage, []byte(out))
			case "loop_subscribe":
				rid, _ := m["request_id"].(string)
				lid, _ := m["loop_id"].(string)
				out := fmt.Sprintf(
					`{"type":"loop_subscribe_response","request_id":%q,"loop_id":%q,"success":true}`,
					rid,
					lid,
				)
				conn.WriteMessage(websocket.TextMessage, []byte(out))
			default:
				conn.WriteMessage(websocket.TextMessage, msg)
			}
		}
	}))
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), DefaultConfig())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	loopID, err := BootstrapLoopSession(ctx, client, "", "/tmp/ws", DefaultConfig())
	if err != nil {
		t.Fatalf("BootstrapLoopSession: %v", err)
	}
	if loopID != "test-loop-456" {
		t.Errorf("loop_id: %s", loopID)
	}
}

func TestBootstrapLoopSession_resumeLoop(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			if err := json.Unmarshal(msg, &m); err != nil {
				continue
			}
			typ, _ := m["type"].(string)

			switch typ {
			case "daemon_ready":
				conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"daemon_ready","state":"ready"}`))
			case "loop_subscribe":
				rid, _ := m["request_id"].(string)
				lid, _ := m["loop_id"].(string)
				out := fmt.Sprintf(
					`{"type":"loop_subscribe_response","request_id":%q,"loop_id":%q,"success":true}`,
					rid,
					lid,
				)
				conn.WriteMessage(websocket.TextMessage, []byte(out))
			default:
				conn.WriteMessage(websocket.TextMessage, msg)
			}
		}
	}))
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), DefaultConfig())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	loopID, err := BootstrapLoopSession(ctx, client, "existing-thread", "/tmp/ws", DefaultConfig())
	if err != nil {
		t.Fatalf("BootstrapLoopSession: %v", err)
	}
	if loopID != "existing-thread" {
		t.Errorf("loop_id: %s", loopID)
	}
}

func TestWaitDaemonReady_Timeout(t *testing.T) {
	ch := make(chan interface{}, 1)
	close(ch) // closed channel, no messages

	ctx := context.Background()
	err := WaitDaemonReady(ctx, ch, 100*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestWaitDaemonReady_ContextCancelled(t *testing.T) {
	ch := make(chan interface{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := WaitDaemonReady(ctx, ch, 5*time.Second)
	if err == nil {
		t.Error("expected context cancelled error")
	}
}

func TestWaitDaemonReady_NotReady(t *testing.T) {
	ch := make(chan interface{}, 1)
	ch <- DaemonReadyResponse{BaseMessage: BaseMessage{Type: "daemon_ready"}, State: "initializing"}

	err := WaitDaemonReady(context.Background(), ch, 1*time.Second)
	if err == nil {
		t.Error("expected error for non-ready state")
	}
}

func TestWaitDaemonReady_Ready(t *testing.T) {
	ch := make(chan interface{}, 1)
	ch <- DaemonReadyResponse{BaseMessage: BaseMessage{Type: "daemon_ready"}, State: "ready"}

	err := WaitDaemonReady(context.Background(), ch, 1*time.Second)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWaitDaemonReady_RawMap(t *testing.T) {
	ch := make(chan interface{}, 1)
	ch <- map[string]interface{}{"type": "daemon_ready", "state": "ready"}

	err := WaitDaemonReady(context.Background(), ch, 1*time.Second)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWaitThreadStatusWithID(t *testing.T) {
	ch := make(chan interface{}, 1)
	ch <- StatusResponse{
		BaseMessage: BaseMessage{Type: "status"},
		ThreadID:    "thread-abc",
	}

	status, err := WaitThreadStatusWithID(context.Background(), ch, 1*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.ThreadID != "thread-abc" {
		t.Errorf("thread_id: %s", status.ThreadID)
	}
}

func TestWaitThreadStatusWithID_ErrorResponse(t *testing.T) {
	ch := make(chan interface{}, 1)
	ch <- ErrorResponse{
		BaseMessage: BaseMessage{Type: "error"},
		Code:        "not_found",
		Message:     "thread not found",
	}

	_, err := WaitThreadStatusWithID(context.Background(), ch, 1*time.Second)
	if err == nil {
		t.Error("expected error from daemon error response")
	}
}

func TestWaitSubscriptionConfirmed(t *testing.T) {
	ch := make(chan interface{}, 1)
	ch <- SubscriptionConfirmedResponse{
		BaseMessage: BaseMessage{Type: "subscription_confirmed"},
		LoopID:      "thread-abc",
		Verbosity:   "normal",
	}

	err := WaitSubscriptionConfirmed(context.Background(), ch, "thread-abc", "normal", 1*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWaitSubscriptionConfirmed_Mismatch(t *testing.T) {
	ch := make(chan interface{}, 1)
	ch <- SubscriptionConfirmedResponse{
		BaseMessage: BaseMessage{Type: "subscription_confirmed"},
		ThreadID:    "different-thread",
	}

	err := WaitSubscriptionConfirmed(context.Background(), ch, "thread-abc", "normal", 100*time.Millisecond)
	if err == nil {
		t.Error("expected timeout for mismatched thread_id")
	}
}

func TestConnectWithRetries_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(testEchoHandler))
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := ConnectWithRetries(ctx, client, 3, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("ConnectWithRetries: %v", err)
	}
	if !client.IsConnected() {
		t.Error("should be connected")
	}
	client.Close()
}

func TestConnectWithRetries_Failure(t *testing.T) {
	client := NewClient("ws://localhost:59999", nil) // port that nobody listens on
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := ConnectWithRetries(ctx, client, 3, 50*time.Millisecond)
	if err == nil {
		t.Error("expected connection failure")
	}
}

func TestConnectWithRetries_Defaults(t *testing.T) {
	// Test with zero values uses defaults
	ts := httptest.NewServer(http.HandlerFunc(testEchoHandler))
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := ConnectWithRetries(ctx, client, 0, 0)
	if err != nil {
		t.Fatalf("ConnectWithRetries with defaults: %v", err)
	}
	client.Close()
}
