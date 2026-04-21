package soothe

// Event namespace constants matching the Soothe daemon wire protocol.
// Format: soothe.<domain>.<component>.<action>

// Plan events
const (
	EventPlanCreated      = "soothe.cognition.plan.created"
	EventPlanStepStarted  = "soothe.cognition.plan.step.started"
	EventPlanStepCompleted = "soothe.cognition.plan.step.completed"
)

// Browser subagent events
const (
	EventBrowserStarted     = "soothe.capability.browser.started"
	EventBrowserCompleted   = "soothe.capability.browser.completed"
	EventBrowserStepRunning = "soothe.capability.browser.step.running"
	EventBrowserCDPConnecting = "soothe.capability.browser.cdp.connecting"
)

// Claude subagent events
const (
	EventClaudeStarted      = "soothe.capability.claude.started"
	EventClaudeTextRunning  = "soothe.capability.claude.text.running"
	EventClaudeToolRunning  = "soothe.capability.claude.tool.running"
	EventClaudeCompleted    = "soothe.capability.claude.completed"
)

// Research subagent events
const (
	EventResearchStarted           = "soothe.capability.research.started"
	EventResearchCompleted         = "soothe.capability.research.completed"
	EventResearchJudgementReporting = "soothe.capability.research.judgement.reporting"
	EventResearchInternalLLM       = "soothe.capability.research.internal_llm.run"
)

// Thread lifecycle events
const (
	EventThreadStarted   = "soothe.lifecycle.thread.started"
	EventThreadResumed   = "soothe.lifecycle.thread.resumed"
	EventThreadCompleted = "soothe.lifecycle.thread.completed"
	EventThreadError     = "soothe.lifecycle.thread.error"
)

// Tool events
const (
	EventToolStarted   = "soothe.tool.execution.started"
	EventToolCompleted = "soothe.tool.execution.completed"
	EventToolError     = "soothe.tool.execution.error"
)

// Agent loop events
const (
	EventAgentLoopStarted   = "soothe.cognition.agent_loop.started"
	EventAgentLoopIterated  = "soothe.cognition.agent_loop.iterated"
	EventAgentLoopCompleted = "soothe.cognition.agent_loop.completed"
)

// Message protocol events
const (
	EventMessageReceived = "soothe.protocol.message.received"
	EventMessageSent     = "soothe.protocol.message.sent"
)

// Output events
const (
	EventChitchatResponse = "soothe.output.chitchat.responded"
	EventFinalReport      = "soothe.output.autonomous.final_report.reported"
)

// ParseNamespace splits a 4-segment event namespace into its components.
// Returns (domain, component, action, ok).
func ParseNamespace(ns string) (domain, component, action string, ok bool) {
	// Expected: soothe.<domain>.<component>.<action>
	// We split on "." and take indices 1,2,3
	parts := splitNamespace(ns)
	if len(parts) < 4 || parts[0] != "soothe" {
		return "", "", "", false
	}
	return parts[1], parts[2], parts[3], true
}

func splitNamespace(ns string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(ns); i++ {
		if ns[i] == '.' {
			parts = append(parts, ns[start:i])
			start = i + 1
		}
	}
	parts = append(parts, ns[start:])
	return parts
}

// ClassifyEventVerbosity returns the VerbosityTier for a given event type string.
// This mirrors soothe_sdk.ux.classification.classify_event_to_tier.
func ClassifyEventVerbosity(eventTypeOrNamespace string) VerbosityTier {
	domain, component, _, ok := ParseNamespace(eventTypeOrNamespace)
	if !ok {
		// Try matching on the full string
		return classifyByEventTypeString(eventTypeOrNamespace)
	}
	return classifyByDomainAndComponent(domain, component, eventTypeOrNamespace)
}

func classifyByDomainAndComponent(domain, component, full string) VerbosityTier {
	switch domain {
	case "lifecycle":
		return TierDetailed
	case "protocol":
		return TierDetailed
	case "cognition":
		return TierNormal
	case "tool":
		return TierInternal
	case "capability":
		// Capability subagents: progress events -> NORMAL, others -> DETAILED
		return classifyCapabilityEvent(full)
	case "output":
		return TierQuiet
	default:
		return TierNormal
	}
}

func classifyCapabilityEvent(full string) VerbosityTier {
	_, _, action, _ := ParseNamespace(full)
	switch action {
	case "started", "completed":
		return TierNormal
	default:
		return TierDetailed
	}
}

func classifyByEventTypeString(s string) VerbosityTier {
	switch s {
	case EventChitchatResponse, EventFinalReport, EventThreadError:
		return TierQuiet
	case EventPlanCreated, EventPlanStepStarted, EventPlanStepCompleted,
		EventAgentLoopStarted, EventAgentLoopIterated,
		EventBrowserStarted, EventBrowserCompleted,
		EventClaudeStarted, EventClaudeCompleted,
		EventResearchStarted, EventResearchCompleted,
		EventResearchJudgementReporting:
		return TierNormal
	case EventAgentLoopCompleted:
		return TierQuiet
	default:
		return TierNormal
	}
}

// IsCompletionEvent checks if an event namespace signals thread completion.
func IsCompletionEvent(namespace string) bool {
	_, _, action, ok := ParseNamespace(namespace)
	if !ok {
		return false
	}
	return action == "completed" || namespace == EventThreadCompleted
}

// IsSubagentProgressEvent checks if an event is a subagent progress event.
func IsSubagentProgressEvent(namespace string) bool {
	switch namespace {
	case EventBrowserStarted, EventBrowserCompleted,
		EventClaudeStarted, EventClaudeCompleted,
		EventResearchStarted, EventResearchCompleted,
		EventResearchJudgementReporting:
		return true
	default:
		return false
	}
}

// EssentialEventTypes are always processed regardless of verbosity.
var EssentialEventTypes = map[string]bool{
	EventThreadCompleted:    true,
	EventThreadError:        true,
	EventChitchatResponse:   true,
	EventFinalReport:        true,
	EventPlanCreated:        true,
	EventPlanStepStarted:    true,
	EventPlanStepCompleted:  true,
	EventAgentLoopStarted:   true,
	EventAgentLoopIterated:  true,
	EventAgentLoopCompleted: true,
	EventBrowserStarted:     true,
	EventBrowserCompleted:   true,
	EventClaudeStarted:      true,
	EventClaudeCompleted:    true,
	EventResearchStarted:    true,
	EventResearchCompleted:  true,
	EventResearchJudgementReporting: true,
}