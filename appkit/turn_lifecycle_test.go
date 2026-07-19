package appkit

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"
	"time"

	soothe "github.com/mirasoth/soothe-client-go"
)

// ---------------------------------------------------------------------------
// Classifier
// ---------------------------------------------------------------------------

func TestClassifier_StatusIdleAfterContent_OptIn(t *testing.T) {
	cl := NewEventClassifier(ClassifierConfig{
		DeliverablePhases:         soothe.DefaultDeliverablePhases(),
		TreatStatusIdleAsComplete: true,
	})
	accumulated := "Hello, this is enough text."
	r := cl.Classify(soothe.StatusResponse{State: "idle", LoopID: "L1"}, accumulated)
	if r.Terminal != ChatEventDeliverableComplete {
		t.Fatalf("expected deliverable, got %v", r.Terminal)
	}
	if r.CompletionEvent != "status.idle" {
		t.Errorf("completion event: %q", r.CompletionEvent)
	}
}

func TestClassifier_StatusIdleNoContent_Ignored(t *testing.T) {
	cl := NewEventClassifier(ClassifierConfig{
		DeliverablePhases:         soothe.DefaultDeliverablePhases(),
		TreatStatusIdleAsComplete: true,
	})
	r := cl.Classify(soothe.StatusResponse{State: "idle"}, "")
	if r.Terminal != ChatEventContinue {
		t.Fatalf("expected continue with empty content, got %v", r.Terminal)
	}
}

func TestClassifier_StatusIdle_DefaultOff(t *testing.T) {
	cl := defaultClassifier()
	r := cl.Classify(soothe.StatusResponse{State: "idle"}, "Hello, this is enough text.")
	if r.Terminal != ChatEventContinue {
		t.Fatalf("default config must not complete on status idle, got %v", r.Terminal)
	}
}

func TestClassifier_SkipsSubscriptionMetadataMap(t *testing.T) {
	cl := defaultClassifier()
	msg := soothe.EventMessage{
		Namespace: []string{"soothe", "system"},
		Data: map[string]interface{}{
			"loop_id":    "L1",
			"latest_seq": float64(3),
		},
	}
	r := cl.Classify(msg, "")
	if r.Content != "" {
		t.Errorf("metadata map must not yield content, got %q", r.Content)
	}
	if r.Terminal != ChatEventContinue {
		t.Errorf("expected continue, got %v", r.Terminal)
	}
}

func TestClassifier_PlanDirect_NotDeliverableByDefault(t *testing.T) {
	cl := defaultClassifier()
	msg := eventMessageFromJSON(t, deliverableEvent("plan_direct", "I will count the files next."))
	r := cl.Classify(msg, "")
	if r.Terminal == ChatEventDeliverableComplete {
		t.Fatal("plan_direct must not be deliverable under DefaultDeliverablePhases")
	}
}

func TestClassifier_GoalCompletion_Deliverable(t *testing.T) {
	cl := defaultClassifier()
	msg := eventMessageFromJSON(t, deliverableEvent("goal_completion", "The answer is forty-two."))
	r := cl.Classify(msg, "")
	if r.Terminal != ChatEventDeliverableComplete {
		t.Fatalf("goal_completion should be deliverable, got %v", r.Terminal)
	}
}

func TestClassifier_StreamEnd_OptInGated(t *testing.T) {
	cl := NewEventClassifier(ClassifierConfig{
		DeliverablePhases:        soothe.DefaultDeliverablePhases(),
		TreatStreamEndAsComplete: true,
	})
	gate := &TurnLifecycleGate{}
	accumulated := "Dali weather is sunny, twenty-eight degrees Celsius today."

	// Premature stream.end before running/progress must not complete.
	end := eventMessageFromJSON(t, streamEndTurnEvent())
	r := cl.ClassifyTurn(end, accumulated, gate)
	if r.Terminal != ChatEventContinue {
		t.Fatalf("premature stream.end must continue, got %v", r.Terminal)
	}

	gate.Observe(soothe.StatusResponse{State: "running", LoopID: "L1"})
	chunk := eventMessageFromJSON(t, streamingChunkEvent("Dali weather"))
	_ = cl.ClassifyTurn(chunk, "", gate)
	if !gate.SawTurnProgress {
		t.Fatal("messages chunk should mark turn progress")
	}

	r = cl.ClassifyTurn(end, accumulated, gate)
	if r.Terminal != ChatEventDeliverableComplete {
		t.Fatalf("gated stream.end should deliver, got %v", r.Terminal)
	}
	if r.CompletionEvent != soothe.STREAM_END {
		t.Errorf("completion event: %q", r.CompletionEvent)
	}
	if !cl.IsDeliverableCompletionEvent(r.CompletionEvent) {
		t.Error("soothe.stream.end must be a deliverable completion event")
	}
}

func TestClassifier_StreamEnd_DefaultOff(t *testing.T) {
	cl := defaultClassifier()
	gate := &TurnLifecycleGate{SawRunning: true, SawTurnProgress: true, SawStreamPayload: true}
	r := cl.ClassifyTurn(eventMessageFromJSON(t, streamEndTurnEvent()), "enough accumulated reply text here", gate)
	if r.Terminal != ChatEventContinue {
		t.Fatalf("default must ignore stream.end, got %v", r.Terminal)
	}
}

func TestClassifier_StatusIdle_GatedIgnoresPreRunning(t *testing.T) {
	cl := NewEventClassifier(ClassifierConfig{
		DeliverablePhases:         soothe.DefaultDeliverablePhases(),
		TreatStatusIdleAsComplete: true,
		GateTurnEndSignals:        true,
	})
	gate := &TurnLifecycleGate{}
	// Continue-thread stub idle before running (渭南).
	r := cl.ClassifyTurn(soothe.StatusResponse{State: "idle"}, "I'll respond to your request using prior context.", gate)
	if r.Terminal != ChatEventContinue {
		t.Fatalf("pre-running idle must continue when gated, got %v", r.Terminal)
	}

	gate.Observe(soothe.StatusResponse{State: "running"})
	_ = cl.ClassifyTurn(eventMessageFromJSON(t, streamingChunkEvent("full weather reply text")), "", gate)
	r = cl.ClassifyTurn(soothe.StatusResponse{State: "idle"}, "full weather reply text for Dali county", gate)
	if r.Terminal != ChatEventDeliverableComplete {
		t.Fatalf("post-running idle should deliver when gated, got %v", r.Terminal)
	}
}

func TestTurnRunner_StreamEndSoftComplete(t *testing.T) {
	// TurnBoundary ends the turn even when the classifier has no stream.end flags.
	store := newMemStore()
	running := soothe.StatusResponse{State: "running", LoopID: "loop-1"}
	chunk := eventMessageFromJSON(t, streamingChunkEvent("enough accumulated reply text"))
	end := eventMessageFromJSON(t, streamEndTurnEvent())
	fake := newFakeClient(running, chunk, end)
	pool := newTestPool(t, store, fake)
	tr := NewTurnRunner(pool, NewQueryGate(), defaultClassifier(), store, NewSSEBroadcaster(), TurnConfig{
		QueryTimeout: 5 * time.Second,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := tr.Execute(ctx, "s1", "hi", "u", "ws", nil, nil); err != nil {
		t.Fatalf("stream.end soft-complete: %v", err)
	}
	msgs := store.messages("s1")
	if len(msgs) == 0 || msgs[0].Role != "assistant" {
		t.Fatalf("expected assistant persist, got %+v", msgs)
	}
	if ce, _ := msgs[0].Metadata["completion_event"].(string); ce != soothe.STREAM_END {
		t.Errorf("completion_event=%q", ce)
	}
}

func TestTurnRunner_GatedIdleSoftComplete(t *testing.T) {
	store := newMemStore()
	running := soothe.StatusResponse{State: "running", LoopID: "loop-1"}
	chunk := eventMessageFromJSON(t, streamingChunkEvent("enough accumulated reply text"))
	idle := soothe.StatusResponse{State: "idle", LoopID: "loop-1"}
	fake := newFakeClient(running, chunk, idle)
	pool := newTestPool(t, store, fake)
	tr := NewTurnRunner(pool, NewQueryGate(), defaultClassifier(), store, NewSSEBroadcaster(), TurnConfig{
		QueryTimeout: 5 * time.Second,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := tr.Execute(ctx, "s1", "hi", "u", "ws", nil, nil); err != nil {
		t.Fatalf("gated idle soft-complete: %v", err)
	}
	msgs := store.messages("s1")
	if len(msgs) == 0 || msgs[0].Role != "assistant" {
		t.Fatalf("expected assistant persist, got %+v", msgs)
	}
	if ce, _ := msgs[0].Metadata["completion_event"].(string); ce != TurnEndIdle {
		t.Errorf("completion_event=%q want %s", ce, TurnEndIdle)
	}
}

func TestTurnBoundary_IgnoresPreRunningIdle(t *testing.T) {
	b := &TurnBoundary{}
	ended, _ := b.Feed(soothe.StatusResponse{State: "idle"})
	if ended {
		t.Fatal("pre-running idle must not end the turn")
	}
	b.Feed(soothe.StatusResponse{State: "running"})
	chunk := eventMessageFromJSON(t, streamingChunkEvent("progress"))
	b.Feed(chunk)
	ended, reason := b.Feed(soothe.StatusResponse{State: "idle"})
	if !ended || reason != TurnEndIdle {
		t.Fatalf("got ended=%v reason=%q", ended, reason)
	}
}

// ---------------------------------------------------------------------------
// Idle timeout
// ---------------------------------------------------------------------------

func TestTurnRunner_IdleTimeout(t *testing.T) {
	store := newMemStore()
	fake := newFakeClient() // no events
	pool := newTestPool(t, store, fake)
	tr := NewTurnRunner(pool, NewQueryGate(), defaultClassifier(), store, NewSSEBroadcaster(), TurnConfig{
		QueryTimeout: 5 * time.Second,
		IdleTimeout:  40 * time.Millisecond,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := tr.Execute(ctx, "s1", "hi", "u", "ws", nil, nil)
	if !errors.Is(err, ErrIdleTimeout) {
		t.Fatalf("expected ErrIdleTimeout, got %v", err)
	}
}

func TestTurnRunner_IdleTimeoutResetsOnEvent(t *testing.T) {
	store := newMemStore()
	chunk := eventMessageFromJSON(t, streamingChunkEvent("partial text here"))
	final := eventMessageFromJSON(t, deliverableEvent("text_completion", "partial text here final answer ok"))
	fake := newFakeClient(chunk, final)
	pool := newTestPool(t, store, fake)
	tr := NewTurnRunner(pool, NewQueryGate(), defaultClassifier(), store, NewSSEBroadcaster(), TurnConfig{
		QueryTimeout: 5 * time.Second,
		IdleTimeout:  80 * time.Millisecond,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := tr.Execute(ctx, "s1", "hi", "u", "ws", nil, nil); err != nil {
		t.Fatalf("execute: %v", err)
	}
}

func TestTurnRunner_IdleDisabledByDefault(t *testing.T) {
	store := newMemStore()
	fake := newFakeClient()
	pool := newTestPool(t, store, fake)
	tr := NewTurnRunner(pool, NewQueryGate(), defaultClassifier(), store, NewSSEBroadcaster(), TurnConfig{
		QueryTimeout: 80 * time.Millisecond,
		// IdleTimeout unset
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := tr.Execute(ctx, "s1", "hi", "u", "ws", nil, nil)
	if !errors.Is(err, ErrQueryTimeout) {
		t.Fatalf("expected ErrQueryTimeout when idle disabled, got %v", err)
	}
}

func TestTurnRunner_IdleFloorWithAttachments(t *testing.T) {
	store := newMemStore()
	fake := newFakeClient()
	pool := newTestPool(t, store, fake)
	tr := NewTurnRunner(pool, NewQueryGate(), defaultClassifier(), store, NewSSEBroadcaster(), TurnConfig{
		QueryTimeout:                  5 * time.Second,
		IdleTimeout:                   20 * time.Millisecond,
		MinIdleTimeoutWithAttachments: 70 * time.Millisecond,
	})
	atts := []map[string]interface{}{{"mime_type": "image/jpeg", "data": "aaaa"}}
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := tr.Execute(ctx, "s1", "describe", "u", "ws", atts, nil)
	elapsed := time.Since(start)
	if !errors.Is(err, ErrIdleTimeout) {
		t.Fatalf("expected ErrIdleTimeout, got %v", err)
	}
	if elapsed < 60*time.Millisecond {
		t.Fatalf("expected attachment floor (~70ms), elapsed %v", elapsed)
	}
}

func TestTurnRunner_IdleNotFiredOnDeliverable(t *testing.T) {
	store := newMemStore()
	final := eventMessageFromJSON(t, deliverableEvent("text_completion", "This is a substantive final answer."))
	fake := newFakeClient(final)
	pool := newTestPool(t, store, fake)
	tr := NewTurnRunner(pool, NewQueryGate(), defaultClassifier(), store, NewSSEBroadcaster(), TurnConfig{
		QueryTimeout: 2 * time.Second,
		IdleTimeout:  200 * time.Millisecond,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := tr.Execute(ctx, "s1", "hi", "u", "ws", nil, nil); err != nil {
		t.Fatalf("execute: %v", err)
	}
}

func TestTurnRunner_IdleSoftComplete(t *testing.T) {
	store := newMemStore()
	chunk := eventMessageFromJSON(t, streamingChunkEvent("enough accumulated reply text"))
	// Emit one chunk then silence until idle.
	fake := newFakeClient(chunk)
	pool := newTestPool(t, store, fake)
	tr := NewTurnRunner(pool, NewQueryGate(), defaultClassifier(), store, NewSSEBroadcaster(), TurnConfig{
		QueryTimeout:  5 * time.Second,
		IdleTimeout:   40 * time.Millisecond,
		OnIdleTimeout: TimeoutPolicySoftComplete,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := tr.Execute(ctx, "s1", "hi", "u", "ws", nil, nil); err != nil {
		t.Fatalf("soft idle should succeed, got %v", err)
	}
	msgs := store.messages("s1")
	if len(msgs) == 0 || msgs[0].Role != "assistant" {
		t.Fatalf("expected soft-complete assistant persist, got %+v", msgs)
	}
}

// ---------------------------------------------------------------------------
// Stream close
// ---------------------------------------------------------------------------

func TestTurnRunner_StreamClose_DefaultFail(t *testing.T) {
	store := newMemStore()
	chunk := eventMessageFromJSON(t, streamingChunkEvent("partial"))
	fake := newFakeClientCloseAfter(chunk)
	pool := newTestPool(t, store, fake)
	tr := NewTurnRunner(pool, NewQueryGate(), defaultClassifier(), store, NewSSEBroadcaster(), TurnConfig{
		QueryTimeout: 2 * time.Second,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := tr.Execute(ctx, "s1", "hi", "u", "ws", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "event stream closed") {
		t.Fatalf("expected stream closed error, got %v", err)
	}
}

func TestTurnRunner_StreamClose_SoftComplete(t *testing.T) {
	store := newMemStore()
	chunk := eventMessageFromJSON(t, streamingChunkEvent("partial accumulated reply text"))
	fake := newFakeClientCloseAfter(chunk)
	pool := newTestPool(t, store, fake)
	tr := NewTurnRunner(pool, NewQueryGate(), defaultClassifier(), store, NewSSEBroadcaster(), TurnConfig{
		QueryTimeout:  2 * time.Second,
		OnStreamClose: StreamCloseSoftComplete,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := tr.Execute(ctx, "s1", "hi", "u", "ws", nil, nil); err != nil {
		t.Fatalf("soft stream close: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Attachments
// ---------------------------------------------------------------------------

func encodeTestJPEG(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 80, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestCompactImageAttachment_DownscalesLarge(t *testing.T) {
	in := encodeTestJPEG(t, 1600, 1200)
	mime, out := CompactImageAttachment("image/jpeg", in, &CompactImageOptions{MaxDim: 768})
	if mime != "image/jpeg" {
		t.Errorf("mime: %s", mime)
	}
	if len(out) >= len(in) {
		t.Errorf("expected smaller payload, in=%d out=%d", len(in), len(out))
	}
	raw, err := base64.StdEncoding.DecodeString(out)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width > 768 || cfg.Height > 768 {
		t.Errorf("dims %dx%d exceed max", cfg.Width, cfg.Height)
	}
}

func TestCompactImageAttachment_PassthroughSmall(t *testing.T) {
	in := encodeTestJPEG(t, 100, 80)
	mime, out := CompactImageAttachment("image/jpeg", in, nil)
	if mime != "image/jpeg" || out != in {
		t.Fatal("small image should pass through unchanged")
	}
}

func TestCompactImageAttachment_PassthroughNonImage(t *testing.T) {
	mime, out := CompactImageAttachment("audio/wav", "AAAA", nil)
	if mime != "audio/wav" || out != "AAAA" {
		t.Fatal("non-image should pass through")
	}
}

func TestCompactImageAttachment_BadBase64(t *testing.T) {
	mime, out := CompactImageAttachment("image/jpeg", "%%%not-b64%%%", nil)
	if mime != "image/jpeg" || out != "%%%not-b64%%%" {
		t.Fatal("bad base64 should pass through")
	}
}

func TestIdleTimeoutForTurn_Floor(t *testing.T) {
	d := idleTimeoutForTurn(TurnConfig{
		IdleTimeout:                   30 * time.Second,
		MinIdleTimeoutWithAttachments: 90 * time.Second,
	}, true)
	if d != 90*time.Second {
		t.Fatalf("got %v", d)
	}
	d2 := idleTimeoutForTurn(TurnConfig{IdleTimeout: 30 * time.Second}, false)
	if d2 != 30*time.Second {
		t.Fatalf("got %v", d2)
	}
}
