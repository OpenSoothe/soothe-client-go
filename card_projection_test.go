package soothe_test

import (
	"testing"

	soothe "github.com/mirasoth/soothe-client-go"
)

func contentOf(card map[string]interface{}) string {
	if card == nil {
		return ""
	}
	v, _ := card["content"].(string)
	return v
}

func idOf(card map[string]interface{}) string {
	if card == nil {
		return ""
	}
	v, _ := card["id"].(string)
	return v
}

func TestParseCardCustomPayloadRejectsLegacy(t *testing.T) {
	if got := soothe.ParseCardCustomPayload(map[string]interface{}{
		"type": "card.created",
		"data": map[string]interface{}{"id": "x", "type": "user", "content": "hi"},
	}); got != nil {
		t.Fatal("legacy bare card.* must be rejected")
	}
}

func TestCardProjectionCreateUpdate(t *testing.T) {
	proj := soothe.NewCardProjection()
	if !proj.Apply(map[string]interface{}{
		"type":    soothe.EventCardCreated,
		"card_id": "a1",
		"data": map[string]interface{}{
			"id":      "a1",
			"type":    "assistant",
			"content": "hel",
		},
	}) {
		t.Fatal("created should apply")
	}
	if got := contentOf(proj.Get("a1")); got != "hel" {
		t.Fatalf("content=%q", got)
	}
	if !proj.Apply(map[string]interface{}{
		"type":    soothe.EventCardUpdated,
		"card_id": "a1",
		"data":    map[string]interface{}{"content": "hello"},
	}) {
		t.Fatal("updated should apply")
	}
	if got := contentOf(proj.Get("a1")); got != "hello" {
		t.Fatalf("content=%q", got)
	}
	if !proj.Apply(map[string]interface{}{"type": soothe.EventCardFinalized, "card_id": "a1", "data": map[string]interface{}{}}) {
		t.Fatal("finalized should apply")
	}
}

func TestCardProjectionReplay(t *testing.T) {
	proj := soothe.NewCardProjection()
	_ = proj.Apply(map[string]interface{}{
		"type": soothe.EventCardCreated,
		"data": map[string]interface{}{"id": "old", "type": "user", "content": "x"},
	})
	if !proj.Apply(map[string]interface{}{"type": soothe.EventCardReplayBegin}) {
		t.Fatal("replay begin")
	}
	if !proj.Replaying() || len(proj.Snapshot()) != 0 {
		t.Fatal("replay should clear")
	}
	_ = proj.Apply(map[string]interface{}{
		"type": soothe.EventCardCreated,
		"data": map[string]interface{}{"id": "new", "type": "user", "content": "y"},
	})
	_ = proj.Apply(map[string]interface{}{"type": soothe.EventCardReplayEnd})
	snap := proj.Snapshot()
	if len(snap) != 1 || idOf(snap[0]) != "new" {
		t.Fatalf("snap=%v", snap)
	}
}

func TestIsTurnProgressChunk_CardFrames(t *testing.T) {
	if !soothe.IsTurnProgressChunk("custom", map[string]interface{}{"type": soothe.EventCardCreated}) {
		t.Fatal("soothe.card.created should count as progress")
	}
	if !soothe.IsTurnProgressChunk("custom", map[string]interface{}{"type": soothe.EventCardUpdated}) {
		t.Fatal("soothe.card.updated should count as progress")
	}
}
