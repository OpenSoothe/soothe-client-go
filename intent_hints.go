package soothe

import (
	"fmt"
	"strings"
)

// Daemon loop_input intent_hint values (direct model turns, no agent graph).

const (
	// IntentHintTextCompletion runs the configured default role on text-only input.
	IntentHintTextCompletion = "text_completion"
	// IntentHintImageToText runs the configured image role (attachments required).
	IntentHintImageToText = "image_to_text"
	// IntentHintOCR runs the configured ocr role (attachments required).
	IntentHintOCR = "ocr"
	// IntentHintEmbed runs the configured embedding role (text-only; JSON vector).
	IntentHintEmbed = "embed"
)

// RemovedIntentHints are legacy loop_input intent_hint values rejected by the daemon.
var RemovedIntentHints = map[string]string{
	"direct_llm": "intent_hint direct_llm is removed; use text_completion (text-only) or image_to_text (with attachments)",
	"quiz":       "intent_hint quiz is removed; omit intent_hint and let intake classify the turn",
}

// ValidateLoopInputIntentHint returns an error for removed legacy hints.
// Pass-through agent hints (e.g. resume_clarification, skill:foo) are allowed.
func ValidateLoopInputIntentHint(hint string) error {
	key := strings.ToLower(strings.TrimSpace(hint))
	if msg, ok := RemovedIntentHints[key]; ok {
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// DefaultDeliverablePhases returns the default set of loop message phases that
// may end a query with a user-facing reply (quiz, goal_completion, direct-model
// hints, and the named direct-model phases).
//
// Loop-assistant phases (see IsLoopAssistantPhase) are a broader set used for
// streaming assistant text. Deliverable phases are the subset that may also
// finish a TurnRunner turn. plan_direct is planning narration only and is not
// included here — final answers use goal_completion, quiz, or direct-model phases.
func DefaultDeliverablePhases() map[string]bool {
	return map[string]bool{
		"quiz":            true,
		"goal_completion": true,
		"direct_model":    true,
		"text_completion": true,
		"image_to_text":   true,
		"ocr":             true,
		"embed":           true,
		"chitchat":        true, // SOCIAL fast-path (Triarch IG-015)
	}
}
