package soothe

import (
	"testing"
	"time"
)

func TestTrackInboundDeliveryAck_CompleteBumpsSeq(t *testing.T) {
	c := NewClient("ws://127.0.0.1:1", nil)
	c.trackInboundDeliveryAck(map[string]interface{}{
		"type":    "complete",
		"loop_id": "loop-ack-1",
	})
	c.mu.Lock()
	recv := c.deliveryRecvSeq["loop-ack-1"]
	c.mu.Unlock()
	if recv != 1 {
		t.Fatalf("deliveryRecvSeq=%d want 1", recv)
	}
	// sendDeliveryAck runs async; wait briefly for acked seq bump.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		acked := c.deliveryAckedSeq["loop-ack-1"]
		c.mu.Unlock()
		if acked == 1 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("deliveryAckedSeq never reached 1")
}

func TestTrackInboundDeliveryAck_StreamEndCustom(t *testing.T) {
	c := NewClient("ws://127.0.0.1:1", nil)
	c.trackInboundDeliveryAck(map[string]interface{}{
		"type":    "event",
		"mode":    "custom",
		"loop_id": "loop-ack-2",
		"data": map[string]interface{}{
			"type":  STREAM_END,
			"scope": "turn",
		},
	})
	c.mu.Lock()
	recv := c.deliveryRecvSeq["loop-ack-2"]
	c.mu.Unlock()
	if recv != 1 {
		t.Fatalf("deliveryRecvSeq=%d want 1 for stream.end", recv)
	}
}
