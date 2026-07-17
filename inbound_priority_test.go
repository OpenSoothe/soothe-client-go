package soothe_test

import (
	"testing"
	"time"

	soothe "github.com/mirasoth/soothe-client-go"
)

func TestInboundFrameDropPriority(t *testing.T) {
	if got := soothe.InboundFrameDropPriority(map[string]interface{}{
		"type": "status", "state": "idle",
	}); got != soothe.DropPriorityCritical {
		t.Fatalf("status idle: %d", got)
	}
	if got := soothe.InboundFrameDropPriority(map[string]interface{}{
		"type": "event", "mode": "updates", "data": map[string]interface{}{},
	}); got != soothe.DropPriorityNormal {
		t.Fatalf("updates: %d", got)
	}
}

func TestPushPendingEvent_PriorityDrop(t *testing.T) {
	c := soothe.NewClient("ws://127.0.0.1:9", nil)
	c.SetInboundMaxSize(3)
	done := make(chan struct{}, 1)
	c.SetStreamDegradedCallback(func(dropped int, reason string) {
		_ = dropped
		_ = reason
		select {
		case done <- struct{}{}:
		default:
		}
	})
	for i := 0; i < 5; i++ {
		c.PushPendingEvent(map[string]interface{}{
			"type": "event", "mode": "updates", "data": map[string]interface{}{"n": i},
		})
	}
	c.PushPendingEvent(map[string]interface{}{"type": "status", "state": "idle", "loop_id": "L1"})
	if c.InboundDropped() == 0 {
		t.Fatal("expected NORMAL frames dropped under overflow")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("expected first-drop callback")
	}
}
