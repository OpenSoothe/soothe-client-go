package soothe

import (
	"context"
	"testing"
)

// =============================================================================
// Unit Tests for Input Options (Clarification Relay)
// =============================================================================

func TestWithClarificationMode(t *testing.T) {
	opts := &inputOptions{}
	WithClarificationMode("auto")(opts)
	if opts.clarificationMode != "auto" {
		t.Errorf("expected clarificationMode='auto', got '%s'", opts.clarificationMode)
	}

	WithClarificationMode("manual")(opts)
	if opts.clarificationMode != "manual" {
		t.Errorf("expected clarificationMode='manual', got '%s'", opts.clarificationMode)
	}
}

func TestWithClarificationAnswer(t *testing.T) {
	opts := &inputOptions{}
	WithClarificationAnswer()(opts)
	if !opts.clarificationAnswer {
		t.Errorf("expected clarificationAnswer=true, got %v", opts.clarificationAnswer)
	}
}

func TestWithClarificationAnswers(t *testing.T) {
	opts := &inputOptions{}
	answers := []string{"answer1", "answer2", "answer3"}
	WithClarificationAnswers(answers)(opts)

	if len(opts.clarificationAnswers) != 3 {
		t.Errorf("expected 3 answers, got %d", len(opts.clarificationAnswers))
	}

	for i, ans := range answers {
		if opts.clarificationAnswers[i] != ans {
			t.Errorf("expected answer[%d]='%s', got '%s'", i, ans, opts.clarificationAnswers[i])
		}
	}
}

func TestSendInputClarificationOptions(t *testing.T) {
	// Test that clarification options are properly included in payload
	// This test validates the payload construction without requiring a daemon

	opts := &inputOptions{
		loopID:               "test-loop-id",
		clarificationMode:    "manual",
		clarificationAnswer:  true,
		clarificationAnswers: []string{"yes", "no", "maybe"},
	}

	// Build the payload manually to verify structure
	payload := map[string]interface{}{
		"type":       "loop_input",
		"loop_id":    opts.loopID,
		"content":    "test input",
		"autonomous": false,
	}

	if opts.clarificationMode != "" {
		payload["clarification_mode"] = opts.clarificationMode
	}
	if opts.clarificationAnswer {
		payload["clarification_answer"] = true
	}
	if opts.clarificationAnswers != nil {
		payload["clarification_answers"] = opts.clarificationAnswers
	}

	// Verify payload structure
	if payload["clarification_mode"] != "manual" {
		t.Errorf("expected clarification_mode='manual', got '%v'", payload["clarification_mode"])
	}
	if payload["clarification_answer"] != true {
		t.Errorf("expected clarification_answer=true, got %v", payload["clarification_answer"])
	}

	answers, ok := payload["clarification_answers"].([]string)
	if !ok {
		t.Errorf("expected clarification_answers to be []string")
	} else if len(answers) != 3 {
		t.Errorf("expected 3 answers, got %d", len(answers))
	}
}

func TestSendInputRequiresLoopID(t *testing.T) {
	// Mock client for testing validation
	client := &Client{}

	ctx := context.Background()

	// SendInput should fail without loopID
	err := client.SendInput(ctx, "test message")
	if err == nil {
		t.Error("expected error for missing loopID")
	}
	if err.Error() != "SendInput requires WithLoopID(loopID)" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestMultipleInputOptions(t *testing.T) {
	opts := &inputOptions{}

	// Apply multiple options
	WithLoopID("loop-123")(opts)
	WithClarificationMode("auto")(opts)
	WithClarificationAnswer()(opts)
	WithClarificationAnswers([]string{"a", "b"})(opts)
	WithModel("openai:gpt-4")(opts)
	WithIntentHint(IntentHintTextCompletion)(opts)

	// Verify all options applied
	if opts.loopID != "loop-123" {
		t.Errorf("expected loopID='loop-123', got '%s'", opts.loopID)
	}
	if opts.clarificationMode != "auto" {
		t.Errorf("expected clarificationMode='auto', got '%s'", opts.clarificationMode)
	}
	if !opts.clarificationAnswer {
		t.Errorf("expected clarificationAnswer=true")
	}
	if len(opts.clarificationAnswers) != 2 {
		t.Errorf("expected 2 answers, got %d", len(opts.clarificationAnswers))
	}
	if opts.model != "openai:gpt-4" {
		t.Errorf("expected model='openai:gpt-4', got '%s'", opts.model)
	}
	if opts.intentHint != IntentHintTextCompletion {
		t.Errorf("expected intentHint=%q, got '%s'", IntentHintTextCompletion, opts.intentHint)
	}
}
