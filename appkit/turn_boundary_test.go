package appkit

import (
	"context"
	"strings"
	"testing"
	"time"

	soothe "github.com/mirasoth/soothe-client-go"
)

func TestTurnBoundary_StreamEndRequiresRunningAndProgress(t *testing.T) {
	b := &TurnBoundary{}
	end := eventMessageFromJSON(t, streamEndTurnEvent())

	ended, _ := b.Feed(end)
	if ended {
		t.Fatal("stream.end before running must not end")
	}

	b.Feed(soothe.StatusResponse{State: "running"})
	ended, _ = b.Feed(end)
	if ended {
		t.Fatal("stream.end before turn progress must not end")
	}

	b.Feed(eventMessageFromJSON(t, streamingChunkEvent("progress text")))
	ended, reason := b.Feed(end)
	if !ended || reason != TurnEndStreamEnd {
		t.Fatalf("got ended=%v reason=%q", ended, reason)
	}
}

func TestTurnBoundary_StoppedAfterRunning(t *testing.T) {
	b := &TurnBoundary{}
	ended, _ := b.Feed(soothe.StatusResponse{State: "stopped"})
	if ended {
		t.Fatal("stopped before running must not end")
	}
	b.Feed(soothe.StatusResponse{State: "running"})
	ended, reason := b.Feed(soothe.StatusResponse{State: "stopped"})
	if !ended || reason != TurnEndStopped {
		t.Fatalf("got ended=%v reason=%q", ended, reason)
	}
}

func TestTurnBoundary_IdempotentAfterEnd(t *testing.T) {
	b := &TurnBoundary{}
	b.Feed(soothe.StatusResponse{State: "running"})
	b.Feed(eventMessageFromJSON(t, streamingChunkEvent("x")))
	ended, reason := b.Feed(eventMessageFromJSON(t, streamEndTurnEvent()))
	if !ended {
		t.Fatal("expected end")
	}
	ended2, reason2 := b.Feed(soothe.StatusResponse{State: "idle"})
	if !ended2 || reason2 != reason {
		t.Fatalf("idempotent end: ended=%v reason=%q want %q", ended2, reason2, reason)
	}
}

func TestIsDaemonTurnEndEvent(t *testing.T) {
	for _, e := range []string{TurnEndStreamEnd, TurnEndIdle, TurnEndStopped} {
		if !IsDaemonTurnEndEvent(e) {
			t.Fatalf("%q should be daemon turn end", e)
		}
	}
	if IsDaemonTurnEndEvent("soothe.protocol.message.goal_completion") {
		t.Fatal("phase completion is not a daemon turn-end event")
	}
}

func TestTurnRunner_BoundaryEmptyContentFails(t *testing.T) {
	store := newMemStore()
	running := soothe.StatusResponse{State: "running", LoopID: "loop-1"}
	// Progress without extractable assistant text (custom plan step).
	progress := eventMessageFromJSON(t, `{
		"proto":"1","type":"event","mode":"custom",
		"data":{"type":"soothe.cognition.strange_loop.step.started","step_id":"s1"},
		"loop_id":"loop-1"
	}`)
	end := eventMessageFromJSON(t, streamEndTurnEvent())
	fake := newFakeClient(running, progress, end)
	pool := newTestPool(t, store, fake)
	tr := NewTurnRunner(pool, NewQueryGate(), defaultClassifier(), store, NewSSEBroadcaster(), TurnConfig{
		QueryTimeout: 5 * time.Second,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := tr.Execute(ctx, "s1", "hi", "u", "ws", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "no assistant content") {
		t.Fatalf("expected empty-content error, got %v", err)
	}
}

func TestTurnRunner_PhaseEarlyCompleteStillWorks(t *testing.T) {
	store := newMemStore()
	final := eventMessageFromJSON(t, deliverableEvent("goal_completion", "The answer is forty-two degrees."))
	fake := newFakeClient(final)
	pool := newTestPool(t, store, fake)
	tr := NewTurnRunner(pool, NewQueryGate(), defaultClassifier(), store, NewSSEBroadcaster(), TurnConfig{
		QueryTimeout: 5 * time.Second,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := tr.Execute(ctx, "s1", "hi", "u", "ws", nil, nil); err != nil {
		t.Fatalf("phase early-complete: %v", err)
	}
	msgs := store.messages("s1")
	if len(msgs) == 0 || msgs[0].Role != "assistant" {
		t.Fatalf("expected assistant, got %+v", msgs)
	}
	ce, _ := msgs[0].Metadata["completion_event"].(string)
	if !strings.Contains(ce, "goal_completion") {
		t.Errorf("completion_event=%q", ce)
	}
}
