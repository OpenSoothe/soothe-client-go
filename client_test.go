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
// Test WebSocket server helpers (RFC-450 protocol-1)
// ---------------------------------------------------------------------------

var upgrader = websocket.Upgrader{}

// testSendHandshake responds to connection_init with a leading status frame
// and a connection_ack reporting readiness_state "ready".
func testSendHandshake(conn *websocket.Conn, m map[string]interface{}) {
	conn.WriteMessage(websocket.TextMessage, []byte(`{"proto":"1","type":"status","state":"idle","input_history":[]}`))
	clientCaps, _ := m["capabilities"].([]interface{})
	daemonCaps := []string{"streaming", "batch", "heartbeat", "receipts"}
	capSet := map[string]bool{}
	for _, c := range clientCaps {
		if s, ok := c.(string); ok {
			capSet[s] = true
		}
	}
	negotiated := []string{}
	for _, c := range daemonCaps {
		if capSet[c] {
			negotiated = append(negotiated, c)
		}
	}
	ack := map[string]interface{}{
		"proto": "1", "type": "connection_ack",
		"result": map[string]interface{}{
			"server_version":        "0.1.0",
			"protocol_version":      "1",
			"capabilities":          negotiated,
			"readiness_state":       "ready",
			"heartbeat_interval_ms": 0,
		},
	}
	b, _ := json.Marshal(ack)
	conn.WriteMessage(websocket.TextMessage, b)
}

// testSendResponse sends a protocol-1 response envelope correlated by id.
func testSendResponse(conn *websocket.Conn, id string, result map[string]interface{}) {
	env := map[string]interface{}{"proto": "1", "type": "response", "result": result, "id": id}
	b, _ := json.Marshal(env)
	conn.WriteMessage(websocket.TextMessage, b)
}

// testSendError sends a protocol-1 error envelope correlated by id.
func testSendError(conn *websocket.Conn, id string, code int, message string) {
	env := map[string]interface{}{"proto": "1", "type": "error", "error": map[string]interface{}{"code": code, "message": message}, "id": id}
	b, _ := json.Marshal(env)
	conn.WriteMessage(websocket.TextMessage, b)
}

// testSendNext sends a protocol-1 next envelope (subscription event) correlated by id.
func testSendNext(conn *websocket.Conn, id string, payload map[string]interface{}) {
	env := map[string]interface{}{"proto": "1", "type": "next", "payload": payload, "id": id}
	b, _ := json.Marshal(env)
	conn.WriteMessage(websocket.TextMessage, b)
}

// isConnectionInit returns true if m is a protocol-1 connection_init envelope.
func isConnectionInit(m map[string]interface{}) bool {
	t, _ := m["type"].(string)
	return t == "connection_init"
}

// testEchoHandler handshakes, then echoes back any message it receives.
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
		var m map[string]interface{}
		if err := json.Unmarshal(msg, &m); err != nil {
			continue
		}
		if isConnectionInit(m) {
			testSendHandshake(conn, m)
			continue
		}
		conn.WriteMessage(websocket.TextMessage, msg)
	}
}

// testFullBootstrapHandler simulates the full daemon handshake + loop lifecycle.
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
		if isConnectionInit(m) {
			testSendHandshake(conn, m)
			continue
		}
		typ, _ := m["type"].(string)
		method, _ := m["method"].(string)
		id, _ := m["id"].(string)
		params, _ := m["params"].(map[string]interface{})
		if params == nil {
			params = map[string]interface{}{}
		}

		switch {
		case typ == "request" && method == "loop_new":
			testSendResponse(conn, id, map[string]interface{}{"loop_id": "test-loop-123", "success": true})
		case typ == "subscribe" && method == "loop_events":
			lid, _ := params["loop_id"].(string)
			payload := map[string]interface{}{"loop_id": lid, "event": "subscribed", "success": true, "client_id": "c1"}
			b, _ := json.Marshal(map[string]interface{}{"proto": "1", "type": "next", "id": id, "payload": payload})
			conn.WriteMessage(websocket.TextMessage, b)
		case typ == "notification" && method == "loop_input":
			lid, _ := params["loop_id"].(string)
			b, _ := json.Marshal(map[string]interface{}{"proto": "1", "type": "status", "state": "running", "loop_id": lid, "workspace": "/tmp"})
			conn.WriteMessage(websocket.TextMessage, b)
		default:
			conn.WriteMessage(websocket.TextMessage, msg)
		}
	}
}

// testNDJSONHandler handshakes, then sends multiple JSON objects in one frame.
func testNDJSONHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	// First message: connection_init → handshake. Second message: trigger → NDJSON.
	conn.ReadMessage()
	testSendHandshake(conn, map[string]interface{}{"capabilities": []interface{}{}})
	conn.ReadMessage()
	conn.WriteMessage(websocket.TextMessage, []byte(
		`{"proto":"1","type":"next","payload":{"namespace":["soothe","output"],"mode":"messages","data":[{"type":"AIMessageChunk","content":"hello","phase":"quiz"},{}],"loop_id":"ndjson-loop"}}`+"\n"+
			`{"proto":"1","type":"status","state":"idle","loop_id":"ndjson-loop"}`,
	))
}

// testRequestResponseHandler handshakes, then answers request envelopes.
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
		if isConnectionInit(m) {
			testSendHandshake(conn, m)
			continue
		}
		typ, _ := m["type"].(string)
		method, _ := m["method"].(string)
		id, _ := m["id"].(string)
		params, _ := m["params"].(map[string]interface{})

		if typ != "request" {
			continue
		}
		switch method {
		case "daemon_status":
			testSendResponse(conn, id, map[string]interface{}{"running": true, "port_live": true, "active_loops": 2})
		case "skills_list":
			testSendResponse(conn, id, map[string]interface{}{"skills": []interface{}{map[string]interface{}{"name": "research"}, map[string]interface{}{"name": "browser"}}})
		case "models_list":
			testSendResponse(conn, id, map[string]interface{}{"models": []interface{}{map[string]interface{}{"id": "gpt-4"}, map[string]interface{}{"id": "claude"}}})
		case "config_get":
			section, _ := params["section"].(string)
			testSendResponse(conn, id, map[string]interface{}{section: map[string]interface{}{"key": "value"}})
		case "daemon_shutdown":
			testSendResponse(conn, id, map[string]interface{}{"status": "acknowledged"})
		case "loop_list":
			testSendResponse(conn, id, map[string]interface{}{"loops": []interface{}{map[string]interface{}{"loop_id": "l1"}, map[string]interface{}{"loop_id": "l2"}}, "total": 2})
		case "invoke_skill":
			testSendResponse(conn, id, map[string]interface{}{"echo": map[string]interface{}{"skill": "test", "status": "ok"}})
		case "error_test":
			testSendError(conn, id, -32603, "test error message")
		default:
			testSendResponse(conn, id, map[string]interface{}{"echoed": method})
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

	msg := map[string]interface{}{"proto": "1", "type": "test", "data": "hello"}
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

	msg := map[string]interface{}{"proto": "1", "type": "test_echo", "data": "world"}
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
		case Envelope:
			if m.Type != "test_echo" {
				t.Errorf("type mismatch: %v", m.Type)
			}
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

func TestExpandWireMessages_EventBatch(t *testing.T) {
	batch := map[string]interface{}{
		"type": "event_batch",
		"events": []interface{}{
			map[string]interface{}{
				"type":    "event",
				"mode":    "messages",
				"data":    []interface{}{map[string]interface{}{"type": "ai", "content": "OK", "phase": "direct_model"}, map[string]interface{}{}},
				"loop_id": "loop-1",
			},
			map[string]interface{}{
				"type":    "status",
				"state":   "idle",
				"loop_id": "loop-1",
			},
		},
	}
	expanded := ExpandWireMessages(batch)
	if len(expanded) != 2 {
		t.Fatalf("expected 2 expanded messages, got %d", len(expanded))
	}
	ev, ok := expanded[0].(EventMessage)
	if !ok {
		t.Fatalf("first expanded message type: %T", expanded[0])
	}
	if ev.Mode != "messages" {
		t.Fatalf("mode: got %q want messages", ev.Mode)
	}
	txt, ok := messagesModeAssistantContent(ev)
	if !ok || txt != "OK" {
		t.Fatalf("assistant content: %q ok=%v", txt, ok)
	}
	st, ok := expanded[1].(StatusResponse)
	if !ok {
		t.Fatalf("second expanded message type: %T", expanded[1])
	}
	if st.State != "idle" {
		t.Fatalf("state: got %q want idle", st.State)
	}
}

func testEventBatchHandler(w http.ResponseWriter, r *http.Request) {
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
		if isConnectionInit(m) {
			testSendHandshake(conn, m)
			continue
		}
		typ, _ := m["type"].(string)
		if typ != "trigger" {
			continue
		}
		payload, err := json.Marshal(map[string]interface{}{
			"type": "event_batch",
			"events": []interface{}{
				map[string]interface{}{
					"type":    "event",
					"mode":    "messages",
					"data":    []interface{}{map[string]interface{}{"type": "ai", "content": "hello", "phase": "quiz"}, map[string]interface{}{}},
					"loop_id": "loop-1",
				},
				map[string]interface{}{"type": "status", "state": "idle", "loop_id": "loop-1"},
			},
		})
		if err != nil {
			return
		}
		if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			return
		}
	}
}

func TestClient_ReceiveMessages_EventBatch(t *testing.T) {
	ts := newTestServer(testEventBatchHandler)
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
	if err := client.SendMessage(ctx, BaseMessage{Type: "trigger"}); err != nil {
		t.Fatalf("send: %v", err)
	}

	var gotEvent, gotIdle bool
	timeout := time.After(3 * time.Second)
	for !(gotEvent && gotIdle) {
		select {
		case raw := <-ch:
			if raw == nil {
				continue
			}
			switch m := raw.(type) {
			case EventMessage:
				if txt, ok := messagesModeAssistantContent(m); ok && txt == "hello" {
					gotEvent = true
				}
			case StatusResponse:
				if m.State == "idle" {
					gotIdle = true
				}
			}
		case <-timeout:
			t.Fatalf("timeout: gotEvent=%v gotIdle=%v", gotEvent, gotIdle)
		}
	}
}

func TestClient_ReceiveMessages_LoopAIMessageEvent(t *testing.T) {
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
	if err := client.SendMessage(ctx, BaseMessage{Type: "trigger"}); err != nil {
		t.Fatalf("send: %v", err)
	}

	timeout := time.After(3 * time.Second)
	for {
		select {
		case raw := <-ch:
			if raw == nil {
				continue
			}
			event, ok := raw.(EventMessage)
			if !ok {
				continue
			}
			loopMsg, ok := event.LoopAIMessage()
			if !ok {
				continue
			}
			if loopMsg.Phase != "quiz" {
				t.Fatalf("unexpected phase: %s", loopMsg.Phase)
			}
			if loopMsg.LoopAIText() != "hello" {
				t.Fatalf("unexpected loop text: %q", loopMsg.LoopAIText())
			}
			return
		case <-timeout:
			t.Fatal("timeout waiting for loop ai message event")
		}
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
	}, "daemon_status", 3*time.Second)
	if err != nil {
		t.Fatalf("RequestResponse: %v", err)
	}
	if resp["running"] != true {
		t.Errorf("running should be true: %v", resp["running"])
	}
}

func TestClient_RequestResponse_Timeout(t *testing.T) {
	cfg := GetCIConfig()
	ts := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		// Handshake so Connect() completes, then never answer the RPC.
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
				testSendHandshake(conn, m)
				continue
			}
			// Got the RPC request — never respond.
			// Use CI-optimized sleep to reduce test time
			time.Sleep(cfg.DefaultTimeout * 2)
		}
	})
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.DefaultTimeout)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	_, err := client.RequestResponse(ctx, map[string]interface{}{
		"type": "daemon_status",
	}, "daemon_status", 500*time.Millisecond)
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
	}, "error_test", 3*time.Second)
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

	err := client.SendInput(ctx, "hello", WithLoopID("t1"), WithModel("openai:gpt-4"))
	if err != nil {
		t.Fatalf("SendInput: %v", err)
	}

	// Verify the echoed message (loop_input is a notification envelope).
	ev, err := client.ReadEvent()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if ev["type"] != "notification" {
		t.Errorf("type: %v", ev["type"])
	}
	params, _ := ev["params"].(map[string]interface{})
	if params == nil {
		t.Fatalf("missing params: %v", ev)
	}
	if params["content"] != "hello" {
		t.Errorf("content: %v", params["content"])
	}
	if params["loop_id"] != "t1" {
		t.Errorf("loop_id: %v", params["loop_id"])
	}
	if params["model"] != "openai:gpt-4" {
		t.Errorf("model: %v", params["model"])
	}
}

func TestClient_SendInput_PreferredSubagent(t *testing.T) {
	ts := newTestServer(testEchoHandler)
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	err := client.SendInput(ctx, "hello", WithLoopID("t1"), WithSubagent("research"))
	if err != nil {
		t.Fatalf("SendInput: %v", err)
	}

	ev, err := client.ReadEvent()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	params, _ := ev["params"].(map[string]interface{})
	if params == nil {
		t.Fatalf("missing params: %v", ev)
	}
	if params["preferred_subagent"] != "research" {
		t.Errorf("preferred_subagent: %v", params["preferred_subagent"])
	}
}

func TestClient_RequestResponse_PreservesRequestID(t *testing.T) {
	ts := newTestServer(testRequestResponseHandler)
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	fixed := "fixed-rid-abc"
	resp, err := client.RequestResponse(ctx, map[string]interface{}{
		"type":       "daemon_status",
		"request_id": fixed,
	}, "daemon_status", 3*time.Second)
	if err != nil {
		t.Fatalf("RequestResponse: %v", err)
	}
	// The caller-supplied request_id is used as the correlation id; the
	// response is matched on it. The result payload carries the RPC data.
	if resp["running"] != true {
		t.Errorf("running should be true: %v", resp["running"])
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
	err := client.SendInput(ctx, "do stuff", WithLoopID("t1"), WithAutonomous(&maxIter))
	if err != nil {
		t.Fatalf("SendInput: %v", err)
	}

	ev, err := client.ReadEvent()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	params, _ := ev["params"].(map[string]interface{})
	if params == nil {
		t.Fatalf("missing params: %v", ev)
	}
	if params["autonomous"] != true {
		t.Errorf("autonomous: %v", params["autonomous"])
	}
	if params["max_iterations"] != float64(5) {
		t.Errorf("max_iterations: %v", params["max_iterations"])
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
	if ev["type"] != "notification" || ev["method"] != "slash_command" {
		t.Errorf("unexpected type/method: %v / %v", ev["type"], ev["method"])
	}
	params, _ := ev["params"].(map[string]interface{})
	if params == nil || params["cmd"] != "/help" {
		t.Errorf("unexpected cmd: %v", params)
	}
}

// testDisconnectHandler handshakes, then on any post-handshake message sends
// a `disconnect` notification (RFC-450 §9.2) and holds the connection open so
// the client can read the frame before any teardown.
func testDisconnectHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	sentDisconnect := false
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
			testSendHandshake(conn, m)
			continue
		}
		// Respond to the first post-handshake frame with a clean disconnect,
		// then keep reading so the socket stays open until the client closes.
		if !sentDisconnect {
			conn.WriteMessage(websocket.TextMessage, []byte(`{"proto":"1","type":"disconnect"}`))
			sentDisconnect = true
		}
	}
}

func TestClient_SendDetach(t *testing.T) {
	ts := newTestServer(testDisconnectHandler)
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	// A real app runs a ReceiveMessages reader; the daemon's `disconnect`
	// notification is read there and consumed by isControlFrame, firing
	// Disconnected(DisconnectClean) rather than being forwarded as an event.
	rctx, rcancel := context.WithCancel(context.Background())
	defer rcancel()
	ch, err := client.ReceiveMessages(rctx)
	if err != nil {
		t.Fatalf("ReceiveMessages: %v", err)
	}

	if err := client.SendDetach(ctx); err != nil {
		t.Fatalf("SendDetach: %v", err)
	}

	select {
	case cause := <-client.Disconnected():
		if cause != DisconnectClean {
			t.Errorf("expected DisconnectClean, got %v", cause)
		}
		// No non-nil application event should have been forwarded for a lifecycle frame.
		select {
		case ev := <-ch:
			if ev != nil {
				t.Errorf("expected no event forwarded for disconnect, got: %v", ev)
			}
		default:
		}
	case <-time.After(2 * time.Second):
		t.Error("expected Disconnected() to fire on disconnect notification")
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
	skills, _ := resp["skills"].([]interface{})
	if len(skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(skills))
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
	models, _ := resp["models"].([]interface{})
	if len(models) != 2 {
		t.Errorf("expected 2 models, got %d", len(models))
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
	echo, _ := resp["echo"].(map[string]interface{})
	if echo == nil || echo["status"] != "ok" {
		t.Errorf("echo: %v", resp)
	}
}

// ---------------------------------------------------------------------------
// WaitForDaemonReady
// ---------------------------------------------------------------------------

func TestClient_WaitForDaemonReady(t *testing.T) {
	// The protocol-1 handshake is performed in Connect(); WaitForDaemonReady
	// returns immediately once the handshake reports readiness "ready".
	ts := newTestServer(testEchoHandler)
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	ev, err := client.WaitForDaemonReady(3 * time.Second)
	if err != nil {
		t.Fatalf("WaitForDaemonReady: %v", err)
	}
	if ev["readiness_state"] != "ready" {
		t.Errorf("readiness_state: %v", ev["readiness_state"])
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
