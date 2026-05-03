package soothe

import "testing"

func TestDecodeMessage_Input_LegacySubagent(t *testing.T) {
	raw := []byte(`{"type":"input","text":"x","subagent":"legacy-name"}`)
	decoded, err := DecodeMessage(raw)
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}
	im, ok := decoded.(InputMessage)
	if !ok {
		t.Fatalf("expected InputMessage, got %T", decoded)
	}
	if im.PreferredSubagent != "legacy-name" {
		t.Fatalf("PreferredSubagent: got %q", im.PreferredSubagent)
	}
}

func TestDecodeMessage_EventWithLoopAIMessage(t *testing.T) {
	raw := []byte(`{"type":"event","thread_id":"t-1","namespace":[],"mode":"messages","data":[{"type":"AIMessageChunk","content":[{"text":"hello"}],"phase":"goal_completion"},{}]}`)
	decoded, err := DecodeMessage(raw)
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}

	event, ok := decoded.(EventMessage)
	if !ok {
		t.Fatalf("expected EventMessage, got %T", decoded)
	}
	msg, ok := event.LoopAIMessage()
	if !ok {
		t.Fatal("expected loop ai message")
	}
	if msg.Phase != "goal_completion" {
		t.Fatalf("expected goal_completion, got %q", msg.Phase)
	}
	if got := msg.LoopAIText(); got != "hello" {
		t.Fatalf("unexpected loop text: %q", got)
	}
}

func TestEventType_CustomAndLegacyFallback(t *testing.T) {
	custom := EventMessage{
		Mode: "custom",
		Data: map[string]interface{}{"type": EventThreadCompleted},
	}
	if got := custom.EventType(); got != EventThreadCompleted {
		t.Fatalf("custom event type mismatch: %q", got)
	}

	legacy := EventMessage{Namespace: EventThreadCompleted}
	if got := legacy.EventType(); got != EventThreadCompleted {
		t.Fatalf("legacy namespace fallback mismatch: %q", got)
	}
}
