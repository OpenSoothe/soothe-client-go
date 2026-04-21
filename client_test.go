package soothe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// ---------------------------------------------------------------------------
// Test WebSocket server helpers
// ---------------------------------------------------------------------------

var upgrader = websocket.Upgrader{}

// testEchoHandler echoes back any message it receives.
func testEchoHandler(w http.ResponseWriter, r *http.Request) {
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
		conn.WriteMessage(websocket.TextMessage, msg)
	}
}

// testFullBootstrapHandler simulates the full daemon handshake.
func testFullBootstrapHandler(w http.ResponseWriter, r *http.Request) {
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
		case "new_thread":
			conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"status","state":"idle","thread_id":"test-thread-123","workspace":"/tmp","new_thread":true}`))
		case "resume_thread":
			tid, _ := m["thread_id"].(string)
			conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"status","state":"idle","thread_id":"`+tid+`","workspace":"/tmp","thread_resumed":true}`))
		case "subscribe_thread":
			tid, _ := m["thread_id"].(string)
			conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"subscription_confirmed","thread_id":"`+tid+`","client_id":"c1","verbosity":"normal"}`))
		default:
			conn.WriteMessage(websocket.TextMessage, msg)
		}
	}
}

// testNDJSONHandler sends multiple JSON objects in one frame.
func testNDJSONHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.ReadMessage() // consume one message
	conn.WriteMessage(websocket.TextMessage, []byte(
		`{"type":"event","namespace":"soothe.output.chitchat.responded","data":{"text":"hello"}}`+"\n"+
			`{"type":"status","state":"idle","thread_id":"ndjson-thread"}`,
	))
}

// testRequestResponseHandler simulates the request-response RPC pattern.
func testRequestResponseHandler(w http.ResponseWriter, r *http.Request) {
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
		rid, _ := m["request_id"].(string)

		switch typ {
		case "daemon_status":
			conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"daemon_status_response","request_id":"`+rid+`","running":true,"port_live":true,"active_threads":2}`))
		case "skills_list":
			conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"skills_list_response","request_id":"`+rid+`","skills":[{"name":"research"},{"name":"browser"}]}`))
		case "models_list":
			conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"models_list_response","request_id":"`+rid+`","models":[{"id":"gpt-4"},{"id":"claude"}]}`))
		case "config_get":
			section, _ := m["section"].(string)
			conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"config_get_response","request_id":"`+rid+`","`+section+`":{"key":"value"}}`))
		case "daemon_shutdown":
			conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"shutdown_ack","request_id":"`+rid+`","status":"acknowledged"}`))
		case "thread_list":
			conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"thread_list_response","request_id":"`+rid+`","threads":[{"thread_id":"t1"},{"thread_id":"t2"}]}`))
		case "invoke_skill":
			conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"invoke_skill_response","request_id":"`+rid+`","skill":"test","status":"ok"}`))
		case "error_test":
			conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","request_id":"`+rid+`","code":"test_error","message":"test error message"}`))
		default:
			conn.WriteMessage(websocket.TextMessage, msg)
		}
	}
}

// newTestServer creates a test server with the given handler.
func newTestServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}

// wsURL converts an HTTP test server URL to a WebSocket URL.
func wsURL(httpURL string) string {
	return "ws" + strings.TrimPrefix(httpURL, "http")
}

// ---------------------------------------------------------------------------
// Client unit tests
// ---------------------------------------------------------------------------

func TestClient_ConnectAndClose(t *testing.T) {
	ts := newTestServer(testEchoHandler)
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if !client.IsConnected() {
		t.Error("should be connected")
	}
	if err := client.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if client.IsConnected() {
		t.Error("should not be connected after close")
	}
}

func TestClient_SendNotConnected(t *testing.T) {
	client := NewClient("ws://localhost:9999", nil)
	err := client.SendMessage(context.Background(), BaseMessage{Type: "test"})
	if err == nil {
		t.Error("expected error when sending on disconnected client")
	}
}

func TestClient_SendReceive(t *testing.T) {
	ts := newTestServer(testEchoHandler)
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	msg := map[string]interface{}{"type": "test", "data": "hello"}
	if err := client.SendMessage(ctx, msg); err != nil {
		t.Fatalf("send: %v", err)
	}

	ev, err := client.ReadEvent()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if ev["type"] != "test" {
		t.Errorf("expected type=test, got %v", ev["type"])
	}
}

func TestClient_ReceiveMessages(t *testing.T) {
	ts := newTestServer(testEchoHandler)
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	ch, err := client.ReceiveMessages(ctx)
	if err != nil {
		t.Fatalf("ReceiveMessages: %v", err)
	}

	msg := map[string]interface{}{"type": "test_echo", "data": "world"}
	if err := client.SendMessage(ctx, msg); err != nil {
		t.Fatalf("send: %v", err)
	}

	select {
	case received := <-ch:
		if received == nil {
			t.Fatal("received nil")
		}
		switch m := received.(type) {
		case map[string]interface{}:
			if m["type"] != "test_echo" {
				t.Errorf("type mismatch: %v", m["type"])
			}
		default:
			// Decoded by protocol, might be a typed message
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for echo")
	}
}

func TestClient_NDJSONReceive(t *testing.T) {
	ts := newTestServer(testNDJSONHandler)
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	ch, err := client.ReceiveMessages(ctx)
	if err != nil {
		t.Fatalf("ReceiveMessages: %v", err)
	}

	// Trigger the NDJSON response
	if err := client.SendMessage(ctx, BaseMessage{Type: "trigger"}); err != nil {
		t.Fatalf("send: %v", err)
	}

	count := 0
	timeout := time.After(3 * time.Second)
	for count < 2 {
		select {
		case msg := <-ch:
			if msg == nil {
				continue
			}
			count++
		case <-timeout:
			t.Fatalf("timeout: only received %d of 2 messages", count)
		}
	}
	if count != 2 {
		t.Errorf("expected 2 messages from NDJSON frame, got %d", count)
	}
}

func TestClient_RequestResponse(t *testing.T) {
	ts := newTestServer(testRequestResponseHandler)
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	resp, err := client.RequestResponse(ctx, map[string]interface{}{
		"type": "daemon_status",
	}, "daemon_status_response", 3*time.Second)
	if err != nil {
		t.Fatalf("RequestResponse: %v", err)
	}
	if resp["type"] != "daemon_status_response" {
		t.Errorf("type mismatch: %v", resp["type"])
	}
	if resp["running"] != true {
		t.Errorf("running should be true: %v", resp["running"])
	}
}

func TestClient_RequestResponse_Timeout(t *testing.T) {
	ts := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		// Read but never respond
		conn.ReadMessage()
		time.Sleep(5 * time.Second)
	})
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	_, err := client.RequestResponse(ctx, map[string]interface{}{
		"type": "daemon_status",
	}, "daemon_status_response", 500*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestClient_RequestResponse_DaemonError(t *testing.T) {
	ts := newTestServer(testRequestResponseHandler)
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	_, err := client.RequestResponse(ctx, map[string]interface{}{
		"type": "error_test",
	}, "some_response", 3*time.Second)
	if err == nil {
		t.Error("expected daemon error")
	}
}

// ---------------------------------------------------------------------------
// High-level API method tests
// ---------------------------------------------------------------------------

func TestClient_SendInput(t *testing.T) {
	ts := newTestServer(testEchoHandler)
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	err := client.SendInput(ctx, "hello", WithThreadID("t1"), WithModel("openai:gpt-4"))
	if err != nil {
		t.Fatalf("SendInput: %v", err)
	}

	// Verify the echoed message
	ev, err := client.ReadEvent()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if ev["type"] != "input" {
		t.Errorf("type: %v", ev["type"])
	}
	if ev["text"] != "hello" {
		t.Errorf("text: %v", ev["text"])
	}
	if ev["thread_id"] != "t1" {
		t.Errorf("thread_id: %v", ev["thread_id"])
	}
	if ev["model"] != "openai:gpt-4" {
		t.Errorf("model: %v", ev["model"])
	}
}

func TestClient_SendInput_Autonomous(t *testing.T) {
	ts := newTestServer(testEchoHandler)
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	maxIter := 5
	err := client.SendInput(ctx, "do stuff", WithAutonomous(&maxIter))
	if err != nil {
		t.Fatalf("SendInput: %v", err)
	}

	ev, err := client.ReadEvent()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if ev["autonomous"] != true {
		t.Errorf("autonomous: %v", ev["autonomous"])
	}
	if ev["max_iterations"] != float64(5) {
		t.Errorf("max_iterations: %v", ev["max_iterations"])
	}
}

func TestClient_SendCommand(t *testing.T) {
	ts := newTestServer(testEchoHandler)
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	if err := client.SendCommand(ctx, "/help"); err != nil {
		t.Fatalf("SendCommand: %v", err)
	}

	ev, err := client.ReadEvent()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if ev["type"] != "command" || ev["cmd"] != "/help" {
		t.Errorf("unexpected: %v", ev)
	}
}

func TestClient_SendNewThread(t *testing.T) {
	ts := newTestServer(testFullBootstrapHandler)
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	if err := client.SendNewThread(ctx, "/tmp/workspace"); err != nil {
		t.Fatalf("SendNewThread: %v", err)
	}

	// The test server responds to new_thread with a status response
	ev, err := client.ReadEvent()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if ev["type"] != "status" {
		t.Errorf("expected status response, got type: %v", ev["type"])
	}
}

func TestClient_SendDetach(t *testing.T) {
	ts := newTestServer(testEchoHandler)
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	if err := client.SendDetach(ctx); err != nil {
		t.Fatalf("SendDetach: %v", err)
	}

	ev, err := client.ReadEvent()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if ev["type"] != "detach" {
		t.Errorf("type: %v", ev["type"])
	}
}

// ---------------------------------------------------------------------------
// RPC convenience method tests
// ---------------------------------------------------------------------------

func TestClient_ListSkills(t *testing.T) {
	ts := newTestServer(testRequestResponseHandler)
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	resp, err := client.ListSkills(ctx, 3*time.Second)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if resp["type"] != "skills_list_response" {
		t.Errorf("type: %v", resp["type"])
	}
}

func TestClient_ListModels(t *testing.T) {
	ts := newTestServer(testRequestResponseHandler)
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	resp, err := client.ListModels(ctx, 3*time.Second)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if resp["type"] != "models_list_response" {
		t.Errorf("type: %v", resp["type"])
	}
}

func TestClient_InvokeSkill(t *testing.T) {
	ts := newTestServer(testRequestResponseHandler)
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	resp, err := client.InvokeSkill(ctx, "research", "search for X", 3*time.Second)
	if err != nil {
		t.Fatalf("InvokeSkill: %v", err)
	}
	if resp["type"] != "invoke_skill_response" {
		t.Errorf("type: %v", resp["type"])
	}
}

// ---------------------------------------------------------------------------
// WaitForDaemonReady / WaitForSubscriptionConfirmed
// ---------------------------------------------------------------------------

func TestClient_WaitForDaemonReady(t *testing.T) {
	ts := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		// Read the daemon_ready request
		conn.ReadMessage()
		// Send ready response
		conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"daemon_ready","state":"ready"}`))
	})
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	if err := client.SendDaemonReady(ctx); err != nil {
		t.Fatalf("SendDaemonReady: %v", err)
	}

	ev, err := client.WaitForDaemonReady(3 * time.Second)
	if err != nil {
		t.Fatalf("WaitForDaemonReady: %v", err)
	}
	if ev["state"] != "ready" {
		t.Errorf("state: %v", ev["state"])
	}
}

// ---------------------------------------------------------------------------
// Connection recovery
// ---------------------------------------------------------------------------

func TestClient_ConnectionRecovery(t *testing.T) {
	ts := newTestServer(testEchoHandler)
	defer ts.Close()

	client1 := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client1.Connect(ctx); err != nil {
		t.Fatalf("connect1: %v", err)
	}
	client1.Close()
	if client1.IsConnected() {
		t.Error("client1 should be disconnected")
	}

	client2 := NewClient(wsURL(ts.URL), nil)
	if err := client2.Connect(ctx); err != nil {
		t.Fatalf("connect2: %v", err)
	}
	if !client2.IsConnected() {
		t.Error("client2 should be connected")
	}
	client2.Close()
}
