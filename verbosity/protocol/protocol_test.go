package protocol

import (
	"encoding/json"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Encode / Decode round-trip tests
// ---------------------------------------------------------------------------

func TestEncodeMessage_Newline(t *testing.T) {
	msg := InputMessage{
		BaseMessage: BaseMessage{RequestID: "test-1", Type: "input"},
		Text:        "hello",
		ThreadID:    "thread-1",
	}
	encoded, err := EncodeMessage(msg)
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	if len(encoded) == 0 || encoded[len(encoded)-1] != '\n' {
		t.Error("encoded message should end with newline")
	}
}

func TestDecodeMessage_Empty(t *testing.T) {
	msg, err := DecodeMessage([]byte{})
	if err != nil || msg != nil {
		t.Errorf("expected nil/nil, got %v %v", msg, err)
	}
}

func TestDecodeMessage_InvalidJSON(t *testing.T) {
	_, err := DecodeMessage([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestRoundTrip_InputMessage(t *testing.T) {
	msg := InputMessage{
		BaseMessage: BaseMessage{RequestID: "r1", Type: "input"},
		Text:        "hello world",
		ThreadID:    "thread-abc",
		Autonomous:  true,
	}
	encoded, err := EncodeMessage(msg)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	data := encoded[:len(encoded)-1]
	decoded, err := DecodeMessage(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := decoded.(InputMessage)
	if !ok {
		t.Fatalf("expected InputMessage, got %T", decoded)
	}
	if got.Text != msg.Text || got.ThreadID != msg.ThreadID || !got.Autonomous {
		t.Errorf("mismatch: got %+v", got)
	}
}

func TestRoundTrip_CommandMessage(t *testing.T) {
	msg := CommandMessage{
		BaseMessage: BaseMessage{Type: "command"},
		Cmd:         "/help",
	}
	encoded, _ := EncodeMessage(msg)
	data := encoded[:len(encoded)-1]
	decoded, err := DecodeMessage(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := decoded.(CommandMessage)
	if !ok {
		t.Fatalf("expected CommandMessage, got %T", decoded)
	}
	if got.Cmd != "/help" {
		t.Errorf("cmd mismatch: %s", got.Cmd)
	}
}

func TestRoundTrip_SubscribeThreadMessage(t *testing.T) {
	msg := SubscribeThreadMessage{
		BaseMessage:      BaseMessage{RequestID: "s1", Type: "subscribe_thread"},
		ThreadID:         "tid-1",
		VerbosityLevel:   "detailed",
	}
	encoded, _ := EncodeMessage(msg)
	data := encoded[:len(encoded)-1]
	decoded, err := DecodeMessage(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := decoded.(SubscribeThreadMessage)
	if !ok {
		t.Fatalf("expected SubscribeThreadMessage, got %T", decoded)
	}
	if got.ThreadID != "tid-1" || got.VerbosityLevel != "detailed" {
		t.Errorf("mismatch: %+v", got)
	}
}

func TestRoundTrip_NewThreadMessage(t *testing.T) {
	msg := NewNewThreadMessage("/tmp/workspace")
	encoded, _ := EncodeMessage(msg)
	data := encoded[:len(encoded)-1]
	decoded, err := DecodeMessage(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := decoded.(NewThreadMessage)
	if !ok {
		t.Fatalf("expected NewThreadMessage, got %T", decoded)
	}
	if got.Workspace != "/tmp/workspace" {
		t.Errorf("workspace mismatch: %s", got.Workspace)
	}
}

func TestRoundTrip_ResumeThreadMessage(t *testing.T) {
	msg := NewResumeThreadMessage("tid-2", "/tmp/ws2")
	encoded, _ := EncodeMessage(msg)
	data := encoded[:len(encoded)-1]
	decoded, err := DecodeMessage(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := decoded.(ResumeThreadMessage)
	if !ok {
		t.Fatalf("expected ResumeThreadMessage, got %T", decoded)
	}
	if got.ThreadID != "tid-2" || got.Workspace != "/tmp/ws2" {
		t.Errorf("mismatch: %+v", got)
	}
}

func TestRoundTrip_EventMessage(t *testing.T) {
	msg := EventMessage{
		BaseMessage: BaseMessage{Type: "event"},
		Namespace:   "soothe.output.chitchat.responded",
		Data:        map[string]interface{}{"text": "Hello"},
		Timestamp:   time.Now(),
	}
	encoded, _ := EncodeMessage(msg)
	data := encoded[:len(encoded)-1]
	decoded, err := DecodeMessage(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := decoded.(EventMessage)
	if !ok {
		t.Fatalf("expected EventMessage, got %T", decoded)
	}
	if got.Namespace != msg.Namespace {
		t.Errorf("namespace mismatch: %s", got.Namespace)
	}
}

func TestRoundTrip_StatusResponse(t *testing.T) {
	msg := StatusResponse{
		BaseMessage: BaseMessage{Type: "status"},
		State:       "idle",
		ThreadID:    "thread-xyz",
		Workspace:   "/tmp/ws",
	}
	encoded, _ := EncodeMessage(msg)
	data := encoded[:len(encoded)-1]
	decoded, err := DecodeMessage(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := decoded.(StatusResponse)
	if !ok {
		t.Fatalf("expected StatusResponse, got %T", decoded)
	}
	if got.ThreadID != "thread-xyz" {
		t.Errorf("thread_id mismatch: %s", got.ThreadID)
	}
}

func TestDecodeStatusResponse_CamelCaseThreadID(t *testing.T) {
	raw := `{"type":"status","state":"idle","threadId":"camel-case-id"}`
	decoded, err := DecodeMessage([]byte(raw))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := decoded.(StatusResponse)
	if !ok {
		t.Fatalf("expected StatusResponse, got %T", decoded)
	}
	if got.ThreadID != "camel-case-id" {
		t.Errorf("expected camelCase thread_id fallback, got: %s", got.ThreadID)
	}
}

func TestRoundTrip_DaemonReadyResponse(t *testing.T) {
	msg := DaemonReadyResponse{
		BaseMessage: BaseMessage{Type: "daemon_ready"},
		State:       "ready",
		Message:     "daemon is ready",
	}
	encoded, _ := EncodeMessage(msg)
	data := encoded[:len(encoded)-1]
	decoded, err := DecodeMessage(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := decoded.(DaemonReadyResponse)
	if !ok {
		t.Fatalf("expected DaemonReadyResponse, got %T", decoded)
	}
	if got.State != "ready" {
		t.Errorf("state mismatch: %s", got.State)
	}
}

func TestRoundTrip_ErrorResponse(t *testing.T) {
	msg := ErrorResponse{
		BaseMessage: BaseMessage{Type: "error"},
		Code:        "internal_error",
		Message:     "something went wrong",
	}
	encoded, _ := EncodeMessage(msg)
	data := encoded[:len(encoded)-1]
	decoded, err := DecodeMessage(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := decoded.(ErrorResponse)
	if !ok {
		t.Fatalf("expected ErrorResponse, got %T", decoded)
	}
	if got.Code != "internal_error" {
		t.Errorf("code mismatch: %s", got.Code)
	}
}

func TestRoundTrip_DaemonStatusResponse(t *testing.T) {
	raw := `{"type":"daemon_status_response","running":true,"port_live":true,"active_threads":3}`
	decoded, err := DecodeMessage([]byte(raw))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := decoded.(DaemonStatusResponse)
	if !ok {
		t.Fatalf("expected DaemonStatusResponse, got %T", decoded)
	}
	if !got.Running || !got.PortLive || got.ActiveThreads != 3 {
		t.Errorf("mismatch: %+v", got)
	}
}

func TestRoundTrip_ShutdownAckResponse(t *testing.T) {
	raw := `{"type":"shutdown_ack","status":"acknowledged"}`
	decoded, err := DecodeMessage([]byte(raw))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := decoded.(ShutdownAckResponse)
	if !ok {
		t.Fatalf("expected ShutdownAckResponse, got %T", decoded)
	}
	if got.Status != "acknowledged" {
		t.Errorf("status mismatch: %s", got.Status)
	}
}

func TestRoundTrip_ThreadListResponse(t *testing.T) {
	raw := `{"type":"thread_list_response","threads":[{"thread_id":"t1"},{"thread_id":"t2"}]}`
	decoded, err := DecodeMessage([]byte(raw))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := decoded.(ThreadListResponse)
	if !ok {
		t.Fatalf("expected ThreadListResponse, got %T", decoded)
	}
	if len(got.Threads) != 2 {
		t.Errorf("expected 2 threads, got %d", len(got.Threads))
	}
}

func TestRoundTrip_SkillsListResponse(t *testing.T) {
	raw := `{"type":"skills_list_response","skills":[{"name":"skill1"},{"name":"skill2"}]}`
	decoded, err := DecodeMessage([]byte(raw))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := decoded.(SkillsListResponse)
	if !ok {
		t.Fatalf("expected SkillsListResponse, got %T", decoded)
	}
	if len(got.Skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(got.Skills))
	}
}

func TestRoundTrip_ModelsListResponse(t *testing.T) {
	raw := `{"type":"models_list_response","models":[{"id":"gpt-4"}]}`
	decoded, err := DecodeMessage([]byte(raw))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := decoded.(ModelsListResponse)
	if !ok {
		t.Fatalf("expected ModelsListResponse, got %T", decoded)
	}
	if len(got.Models) != 1 {
		t.Errorf("expected 1 model, got %d", len(got.Models))
	}
}

func TestRoundTrip_ConfigGetResponse(t *testing.T) {
	raw := `{"type":"config_get_response","providers":{"openai":{"api_key":"***"}}}`
	decoded, err := DecodeMessage([]byte(raw))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := decoded.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", decoded)
	}
	if got["type"] != "config_get_response" {
		t.Errorf("type mismatch: %v", got["type"])
	}
}

func TestRoundTrip_InvokeSkillResponse(t *testing.T) {
	raw := `{"type":"invoke_skill_response","skill":"test","status":"ok"}`
	decoded, err := DecodeMessage([]byte(raw))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := decoded.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", decoded)
	}
	if got["type"] != "invoke_skill_response" {
		t.Errorf("type mismatch: %v", got["type"])
	}
}

func TestDecodeMessage_UnknownType(t *testing.T) {
	raw := `{"type":"future_type","data":"hello"}`
	decoded, err := DecodeMessage([]byte(raw))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := decoded.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map for unknown type, got %T", decoded)
	}
	if got["type"] != "future_type" {
		t.Errorf("type mismatch: %v", got["type"])
	}
}

// ---------------------------------------------------------------------------
// All client→daemon message type decode tests
// ---------------------------------------------------------------------------

func TestDecodeAllClientMessageTypes(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantTyp string
	}{
		{"input", `{"type":"input","text":"hi","thread_id":"t1"}`, "input"},
		{"command", `{"type":"command","cmd":"/help"}`, "command"},
		{"subscribe_thread", `{"type":"subscribe_thread","thread_id":"t1","verbosity":"normal"}`, "subscribe_thread"},
		{"new_thread", `{"type":"new_thread","workspace":"/tmp"}`, "new_thread"},
		{"resume_thread", `{"type":"resume_thread","thread_id":"t1","workspace":"/tmp"}`, "resume_thread"},
		{"daemon_status", `{"type":"daemon_status"}`, "daemon_status"},
		{"daemon_shutdown", `{"type":"daemon_shutdown"}`, "daemon_shutdown"},
		{"config_get", `{"type":"config_get","section":"providers"}`, "config_get"},
		{"thread_list", `{"type":"thread_list"}`, "thread_list"},
		{"thread_get", `{"type":"thread_get","thread_id":"t1"}`, "thread_get"},
		{"thread_messages", `{"type":"thread_messages","thread_id":"t1","limit":50,"offset":0}`, "thread_messages"},
		{"thread_state", `{"type":"thread_state","thread_id":"t1"}`, "thread_state"},
		{"thread_update_state", `{"type":"thread_update_state","thread_id":"t1","values":{}}`, "thread_update_state"},
		{"thread_archive", `{"type":"thread_archive","thread_id":"t1"}`, "thread_archive"},
		{"thread_delete", `{"type":"thread_delete","thread_id":"t1"}`, "thread_delete"},
		{"thread_create", `{"type":"thread_create"}`, "thread_create"},
		{"thread_artifacts", `{"type":"thread_artifacts","thread_id":"t1"}`, "thread_artifacts"},
		{"resume_interrupts", `{"type":"resume_interrupts","thread_id":"t1","resume_payload":{}}`, "resume_interrupts"},
		{"skills_list", `{"type":"skills_list"}`, "skills_list"},
		{"models_list", `{"type":"models_list"}`, "models_list"},
		{"invoke_skill", `{"type":"invoke_skill","skill":"test","args":""}`, "invoke_skill"},
		{"detach", `{"type":"detach"}`, "detach"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoded, err := DecodeMessage([]byte(tt.json))
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			// All decoded messages should have a BaseMessage with Type
			b, _ := json.Marshal(decoded)
			var m map[string]interface{}
			json.Unmarshal(b, &m)
			if m["type"] != tt.wantTyp {
				t.Errorf("type mismatch: got %v, want %s", m["type"], tt.wantTyp)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SplitSootheWirePayload tests
// ---------------------------------------------------------------------------

func TestSplitSootheWirePayload_SingleJSON(t *testing.T) {
	raw := []byte(`{"type":"status","state":"idle"}`)
	lines := SplitSootheWirePayload(raw)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
}

func TestSplitSootheWirePayload_NDJSON(t *testing.T) {
	raw := []byte(`{"type":"status","state":"idle"}` + "\n" + `{"type":"daemon_ready","state":"ready"}`)
	lines := SplitSootheWirePayload(raw)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
}

func TestSplitSootheWirePayload_Empty(t *testing.T) {
	lines := SplitSootheWirePayload([]byte(""))
	if lines != nil {
		t.Errorf("expected nil for empty input, got %v", lines)
	}
}

func TestSplitSootheWirePayload_TrailingNewline(t *testing.T) {
	raw := []byte(`{"type":"status"}` + "\n")
	lines := SplitSootheWirePayload(raw)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
}

func TestSplitSootheWirePayload_WhitespaceOnly(t *testing.T) {
	raw := []byte("  \n  \n  ")
	lines := SplitSootheWirePayload(raw)
	if lines != nil {
		t.Errorf("expected nil for whitespace-only input, got %v", lines)
	}
}

func TestSplitSootheWirePayload_MultipleNewlines(t *testing.T) {
	raw := []byte(`{"a":1}` + "\n\n" + `{"b":2}` + "\n" + `{"c":3}`)
	lines := SplitSootheWirePayload(raw)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
}

// ---------------------------------------------------------------------------
// ExtractSootheThreadID tests
// ---------------------------------------------------------------------------

func TestExtractSootheThreadID_StatusResponse(t *testing.T) {
	id, ok := ExtractSootheThreadID(StatusResponse{
		BaseMessage: BaseMessage{Type: "status"},
		ThreadID:    "abc",
	})
	if !ok || id != "abc" {
		t.Fatalf("got %q %v", id, ok)
	}
}

func TestExtractSootheThreadID_StatusResponseEmpty(t *testing.T) {
	_, ok := ExtractSootheThreadID(StatusResponse{
		BaseMessage: BaseMessage{Type: "status"},
	})
	if ok {
		t.Error("expected false for empty thread_id")
	}
}

func TestExtractSootheThreadID_EventMessage(t *testing.T) {
	id, ok := ExtractSootheThreadID(EventMessage{
		BaseMessage: BaseMessage{Type: "event"},
		ThreadID:    "evt-thread",
	})
	if !ok || id != "evt-thread" {
		t.Fatalf("got %q %v", id, ok)
	}
}

func TestExtractSootheThreadID_EventMessageData(t *testing.T) {
	id, ok := ExtractSootheThreadID(EventMessage{
		BaseMessage: BaseMessage{Type: "event"},
		Data:        map[string]interface{}{"thread_id": "data-thread"},
	})
	if !ok || id != "data-thread" {
		t.Fatalf("got %q %v", id, ok)
	}
}

func TestExtractSootheThreadID_EventMessageDataCamelCase(t *testing.T) {
	id, ok := ExtractSootheThreadID(EventMessage{
		BaseMessage: BaseMessage{Type: "event"},
		Data:        map[string]interface{}{"threadId": "camel-thread"},
	})
	if !ok || id != "camel-thread" {
		t.Fatalf("got %q %v", id, ok)
	}
}

func TestExtractSootheThreadID_Map(t *testing.T) {
	id, ok := ExtractSootheThreadID(map[string]interface{}{
		"thread_id": "map-thread",
	})
	if !ok || id != "map-thread" {
		t.Fatalf("got %q %v", id, ok)
	}
}

func TestExtractSootheThreadID_UnsupportedType(t *testing.T) {
	_, ok := ExtractSootheThreadID("not a message")
	if ok {
		t.Error("expected false for unsupported type")
	}
}

// ---------------------------------------------------------------------------
// Factory function tests
// ---------------------------------------------------------------------------

func TestNewInputMessage(t *testing.T) {
	msg := NewInputMessage("hello", "thread-1")
	if msg.Type != "input" {
		t.Errorf("type: %s", msg.Type)
	}
	if msg.Text != "hello" {
		t.Errorf("text: %s", msg.Text)
	}
	if msg.ThreadID != "thread-1" {
		t.Errorf("thread_id: %s", msg.ThreadID)
	}
	if msg.RequestID == "" {
		t.Error("request_id should be auto-generated")
	}
}

func TestNewSubscribeThreadMessage(t *testing.T) {
	msg := NewSubscribeThreadMessage("tid", "detailed")
	if msg.Type != "subscribe_thread" {
		t.Errorf("type: %s", msg.Type)
	}
	if msg.ThreadID != "tid" {
		t.Errorf("thread_id: %s", msg.ThreadID)
	}
	if msg.VerbosityLevel != "detailed" {
		t.Errorf("verbosity: %s", msg.VerbosityLevel)
	}
}

func TestNewNewThreadMessage(t *testing.T) {
	msg := NewNewThreadMessage("/tmp/ws")
	if msg.Type != "new_thread" {
		t.Errorf("type: %s", msg.Type)
	}
	if msg.Workspace != "/tmp/ws" {
		t.Errorf("workspace: %s", msg.Workspace)
	}
}

func TestNewResumeThreadMessage(t *testing.T) {
	msg := NewResumeThreadMessage("tid", "/tmp/ws")
	if msg.Type != "resume_thread" {
		t.Errorf("type: %s", msg.Type)
	}
	if msg.ThreadID != "tid" {
		t.Errorf("thread_id: %s", msg.ThreadID)
	}
	if msg.Workspace != "/tmp/ws" {
		t.Errorf("workspace: %s", msg.Workspace)
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkEncodeMessage(b *testing.B) {
	msg := InputMessage{
		BaseMessage: BaseMessage{RequestID: "bench-1", Type: "input"},
		Text:        "benchmark message",
		ThreadID:    "thread-bench",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := EncodeMessage(msg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeMessage(b *testing.B) {
	msg := InputMessage{
		BaseMessage: BaseMessage{RequestID: "bench-1", Type: "input"},
		Text:        "benchmark message",
		ThreadID:    "thread-bench",
	}
	encoded, _ := EncodeMessage(msg)
	data := encoded[:len(encoded)-1]
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := DecodeMessage(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}
