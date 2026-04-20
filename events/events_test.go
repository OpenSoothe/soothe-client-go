package events

import (
	"testing"

	"github.com/mirasurf/lepton/soothe-client-go/verbosity"
)

func TestParseNamespace_Valid(t *testing.T) {
	domain, component, action, ok := ParseNamespace("soothe.cognition.plan.created")
	if !ok {
		t.Fatal("expected ok")
	}
	if domain != "cognition" {
		t.Errorf("domain: %s", domain)
	}
	if component != "plan" {
		t.Errorf("component: %s", component)
	}
	if action != "created" {
		t.Errorf("action: %s", action)
	}
}

func TestParseNamespace_Lifecycle(t *testing.T) {
	domain, component, action, ok := ParseNamespace("soothe.lifecycle.thread.completed")
	if !ok {
		t.Fatal("expected ok")
	}
	if domain != "lifecycle" || component != "thread" || action != "completed" {
		t.Errorf("got %s.%s.%s", domain, component, action)
	}
}

func TestParseNamespace_Invalid(t *testing.T) {
	_, _, _, ok := ParseNamespace("invalid")
	if ok {
		t.Error("expected not ok for invalid namespace")
	}
}

func TestParseNamespace_ShortPath(t *testing.T) {
	_, _, _, ok := ParseNamespace("soothe.cognition")
	if ok {
		t.Error("expected not ok for short namespace")
	}
}

func TestParseNamespace_DeepPath(t *testing.T) {
	// soothe.capability.research.internal_llm.run
	domain, component, action, ok := ParseNamespace("soothe.capability.research.internal_llm.run")
	if !ok {
		t.Fatal("expected ok")
	}
	if domain != "capability" || component != "research" || action != "internal_llm" {
		t.Errorf("got %s.%s.%s", domain, component, action)
	}
}

func TestClassifyEventVerbosity_Quiet(t *testing.T) {
	tests := []struct {
		ns   string
		want verbosity.VerbosityTier
	}{
		{EventChitchatResponse, verbosity.TierQuiet},
		{EventFinalReport, verbosity.TierQuiet},
	}
	for _, tt := range tests {
		got := ClassifyEventVerbosity(tt.ns)
		if got != tt.want {
			t.Errorf("ClassifyEventVerbosity(%s) = %d, want %d", tt.ns, got, tt.want)
		}
	}
}

func TestClassifyEventVerbosity_Normal(t *testing.T) {
	tests := []struct {
		ns   string
		want verbosity.VerbosityTier
	}{
		{EventPlanCreated, verbosity.TierNormal},
		{EventPlanStepStarted, verbosity.TierNormal},
		{EventPlanStepCompleted, verbosity.TierNormal},
		{EventBrowserStarted, verbosity.TierNormal},
		{EventBrowserCompleted, verbosity.TierNormal},
		{EventClaudeStarted, verbosity.TierNormal},
		{EventClaudeCompleted, verbosity.TierNormal},
		{EventResearchStarted, verbosity.TierNormal},
		{EventResearchCompleted, verbosity.TierNormal},
	}
	for _, tt := range tests {
		got := ClassifyEventVerbosity(tt.ns)
		if got != tt.want {
			t.Errorf("ClassifyEventVerbosity(%s) = %d, want %d", tt.ns, got, tt.want)
		}
	}
}

func TestClassifyEventVerbosity_Detailed(t *testing.T) {
	tests := []struct {
		ns   string
		want verbosity.VerbosityTier
	}{
		{EventThreadStarted, verbosity.TierDetailed},
		{EventThreadResumed, verbosity.TierDetailed},
		{EventBrowserStepRunning, verbosity.TierDetailed},
		{EventBrowserCDPConnecting, verbosity.TierDetailed},
		{EventClaudeTextRunning, verbosity.TierDetailed},
		{EventClaudeToolRunning, verbosity.TierDetailed},
	}
	for _, tt := range tests {
		got := ClassifyEventVerbosity(tt.ns)
		if got != tt.want {
			t.Errorf("ClassifyEventVerbosity(%s) = %d, want %d", tt.ns, got, tt.want)
		}
	}
}

func TestClassifyEventVerbosity_Internal(t *testing.T) {
	// Tool events are internal tier
	got := ClassifyEventVerbosity(EventToolStarted)
	if got != verbosity.TierInternal {
		t.Errorf("tool started should be internal, got %d", got)
	}
}

func TestIsCompletionEvent(t *testing.T) {
	tests := []struct {
		ns   string
		want bool
	}{
		{EventThreadCompleted, true},
		{"soothe.capability.browser.completed", true},
		{"soothe.cognition.plan.completed", true},
		{EventPlanCreated, false},
		{EventBrowserStarted, false},
		{"invalid", false},
	}
	for _, tt := range tests {
		got := IsCompletionEvent(tt.ns)
		if got != tt.want {
			t.Errorf("IsCompletionEvent(%s) = %v, want %v", tt.ns, got, tt.want)
		}
	}
}

func TestIsSubagentProgressEvent(t *testing.T) {
	tests := []struct {
		ns   string
		want bool
	}{
		{EventBrowserStarted, true},
		{EventBrowserCompleted, true},
		{EventClaudeStarted, true},
		{EventClaudeCompleted, true},
		{EventResearchStarted, true},
		{EventResearchCompleted, true},
		{EventResearchJudgementReporting, true},
		{EventBrowserStepRunning, false},
		{EventPlanCreated, false},
	}
	for _, tt := range tests {
		got := IsSubagentProgressEvent(tt.ns)
		if got != tt.want {
			t.Errorf("IsSubagentProgressEvent(%s) = %v, want %v", tt.ns, got, tt.want)
		}
	}
}

func TestEssentialEventTypes(t *testing.T) {
	essential := []string{
		EventThreadCompleted,
		EventThreadError,
		EventChitchatResponse,
		EventFinalReport,
		EventPlanCreated,
		EventBrowserStarted,
		EventClaudeStarted,
		EventResearchStarted,
	}
	for _, ev := range essential {
		if !EssentialEventTypes[ev] {
			t.Errorf("expected %s to be essential", ev)
		}
	}
	// Non-essential event
	if EssentialEventTypes[EventBrowserStepRunning] {
		t.Error("browser step running should not be essential")
	}
}
