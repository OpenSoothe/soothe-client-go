package soothe

import "testing"

func TestDefaultDeliverablePhases_ExcludesPlanDirect(t *testing.T) {
	phases := DefaultDeliverablePhases()
	if phases["plan_direct"] {
		t.Fatal("plan_direct must not be a default deliverable phase")
	}
}

func TestDefaultDeliverablePhases_IncludesIntentHints(t *testing.T) {
	phases := DefaultDeliverablePhases()
	for _, want := range []string{
		"quiz", "goal_completion",
		"text_completion", "image_to_text", "ocr", "embed", "chitchat",
	} {
		if !phases[want] {
			t.Errorf("DefaultDeliverablePhases missing %q", want)
		}
	}
	for _, absent := range []string{"direct_model", "direct_llm", "trivial"} {
		if phases[absent] {
			t.Fatalf("%s must not be a deliverable phase", absent)
		}
	}
}

func TestIsLoopAssistantPhase_IncludesChitchat(t *testing.T) {
	if !IsLoopAssistantPhase("chitchat") {
		t.Fatal("chitchat must be a loop-assistant phase")
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
