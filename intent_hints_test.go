package soothe

import (
	"context"
	"strings"
	"testing"
)

func TestValidateLoopInputIntentHint_RemovedLegacy(t *testing.T) {
	for _, hint := range []string{"direct_llm", "DIRECT_LLM", " quiz ", "quiz"} {
		err := ValidateLoopInputIntentHint(hint)
		if err == nil {
			t.Fatalf("expected error for hint %q", hint)
		}
	}
}

func TestValidateLoopInputIntentHint_AllowedPassThrough(t *testing.T) {
	for _, hint := range []string{
		IntentHintTextCompletion,
		"resume_clarification",
		"skill:search",
	} {
		if err := ValidateLoopInputIntentHint(hint); err != nil {
			t.Fatalf("hint %q: unexpected error: %v", hint, err)
		}
	}
}

func TestSendInput_RejectsLegacyIntentHint(t *testing.T) {
	client := NewClient("ws://localhost:8765", nil)
	ctx := context.Background()

	err := client.SendInput(ctx, "hello", WithLoopID("loop-1"), WithIntentHint("direct_llm"))
	if err == nil {
		t.Fatal("expected error for direct_llm intent_hint")
	}
	if !strings.Contains(err.Error(), "direct_llm is removed") {
		t.Fatalf("unexpected error: %v", err)
	}
}
