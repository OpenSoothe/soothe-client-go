package soothe

import "testing"

func TestDefaultDeliverablePhases_ExcludesPlanDirect(t *testing.T) {
	phases := DefaultDeliverablePhases()
	if phases["plan_direct"] {
		t.Fatal("plan_direct must not be a default deliverable phase")
	}
}

func TestDefaultDeliverablePhases_IncludesDirectHints(t *testing.T) {
	phases := DefaultDeliverablePhases()
	for _, want := range []string{
		"quiz", "goal_completion", "direct_model",
		"text_completion", "image_to_text", "ocr", "embed",
	} {
		if !phases[want] {
			t.Errorf("DefaultDeliverablePhases missing %q", want)
		}
	}
}

func TestIsLoopAssistantPhase_IncludesPlanDirect(t *testing.T) {
	if !IsLoopAssistantPhase("plan_direct") {
		t.Fatal("plan_direct must be a loop-assistant phase for text extraction")
	}
	if !IsLoopAssistantPhase("goal_completion") {
		t.Fatal("goal_completion must be a loop-assistant phase")
	}
	if IsLoopAssistantPhase("execute_step") {
		t.Fatal("execute_step must not be a loop-assistant phase")
	}
}
