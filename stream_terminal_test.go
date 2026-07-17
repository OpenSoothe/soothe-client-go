package soothe_test

import (
	"testing"

	soothe "github.com/mirasoth/soothe-client-go"
)

func TestInboundNeedsDeliveryAck_Complete(t *testing.T) {
	if !soothe.InboundNeedsDeliveryAck(map[string]interface{}{"type": "complete", "loop_id": "L1"}) {
		t.Fatal("complete should need ack")
	}
}

func TestInboundNeedsDeliveryAck_TurnEndCustom(t *testing.T) {
	ev := map[string]interface{}{
		"type":    "event",
		"loop_id": "L1",
		"mode":    "custom",
		"data": map[string]interface{}{
			"type":  soothe.STREAM_END,
			"scope": "turn",
		},
	}
	if !soothe.InboundNeedsDeliveryAck(ev) {
		t.Fatal("stream.end custom should need ack")
	}
}

func TestStalePendingFrameLabel(t *testing.T) {
	if got := soothe.StalePendingFrameLabel(map[string]interface{}{"type": "connection_ack"}); got != "connection_ack" {
		t.Fatalf("got %q", got)
	}
	if got := soothe.StalePendingFrameLabel(map[string]interface{}{
		"type": "event",
		"mode": "messages",
		"data": []interface{}{},
	}); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestIsTurnProgressChunk(t *testing.T) {
	if !soothe.IsTurnProgressChunk("messages", nil) {
		t.Fatal("messages should count as progress")
	}
	if soothe.IsTurnProgressChunk("custom", map[string]interface{}{
		"type":  soothe.STREAM_END,
		"scope": "turn",
	}) {
		t.Fatal("stream.end should not count as progress")
	}
}
