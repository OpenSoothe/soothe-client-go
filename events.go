package soothe

// Event namespace constants matching the Soothe daemon wire protocol.
// Format: soothe.<domain>.<component>.<action>

// Plan events
const (
	EventPlanCreated       = "soothe.cognition.plan.created"
	EventPlanStepStarted   = "soothe.cognition.plan.step.started"
	EventPlanStepCompleted = "soothe.cognition.plan.step.completed"
	EventPlanStepFailed    = "soothe.cognition.plan.step.failed"
	EventPlanBatchStarted  = "soothe.cognition.plan.batch.started"
	EventPlanReflected     = "soothe.cognition.plan.reflected"
	EventPlanDagSnapshot   = "soothe.cognition.plan.dag_snapshot"
)

// Goal events
const (
	EventGoalCreated           = "soothe.cognition.goal.created"
	EventGoalCompleted         = "soothe.cognition.goal.completed"
	EventGoalFailed            = "soothe.cognition.goal.failed"
	EventGoalDeferred          = "soothe.cognition.goal.deferred"
	EventGoalBatchStarted      = "soothe.cognition.goal.batch.started"
	EventGoalReported          = "soothe.cognition.goal.reported"
	EventGoalDirectivesApplied = "soothe.cognition.goal.directives.applied"
)

// Browser subagent events
const (
	EventBrowserStarted       = "soothe.capability.browser.started"
	EventBrowserCompleted     = "soothe.capability.browser.completed"
	EventBrowserStepRunning   = "soothe.capability.browser.step.running"
	EventBrowserCDPConnecting = "soothe.capability.browser.cdp.connecting"
)

// Claude subagent events
const (
	EventClaudeStarted     = "soothe.capability.claude.started"
	EventClaudeTextRunning = "soothe.capability.claude.text.running"
	EventClaudeToolRunning = "soothe.capability.claude.tool.running"
	EventClaudeCompleted   = "soothe.capability.claude.completed"
)

// Research subagent events
const (
	EventResearchStarted            = "soothe.capability.research.started"
	EventResearchCompleted          = "soothe.capability.research.completed"
	EventResearchJudgementReporting = "soothe.capability.research.judgement.reporting"
	EventResearchInternalLLM        = "soothe.capability.research.internal_llm.run"
)

// Iteration lifecycle events
const (
	EventIterationStarted   = "soothe.lifecycle.iteration.started"
	EventIterationCompleted = "soothe.lifecycle.iteration.completed"
)

// Checkpoint lifecycle events
const (
	EventCheckpointSaved         = "soothe.lifecycle.checkpoint.saved"
	EventCheckpointAnchorCreated = "soothe.lifecycle.checkpoint.anchor.created"
)

// Recovery lifecycle events
const (
	EventRecoveryResumed = "soothe.lifecycle.recovery.resumed"
)

// Loop lifecycle events
const (
	EventLoopCreated         = "soothe.lifecycle.loop.created"
	EventLoopStarted         = "soothe.lifecycle.loop.started"
	EventLoopDetached        = "soothe.lifecycle.loop.detached"
	EventLoopReattached      = "soothe.lifecycle.loop.reattached"
	EventLoopCompleted       = "soothe.lifecycle.loop.completed"
	EventLoopHistoryReplayed = "soothe.lifecycle.loop.history.replayed"
)

// Tool events
const (
	EventToolStarted   = "soothe.tool.execution.started"
	EventToolCompleted = "soothe.tool.execution.completed"
	EventToolError     = "soothe.tool.execution.error"
)

// Stream tool call events (RFC-450, IG-416)
const (
	EventStreamToolCallUpdate  = "soothe.stream.tool_call.update"
	EventToolCallUpdatesBatch = "tool_call_updates_batch"
)

// Agent loop events
const (
	EventAgentLoopStarted       = "soothe.cognition.agent_loop.started"
	EventAgentLoopIterated      = "soothe.cognition.agent_loop.iterated"
	EventAgentLoopCompleted     = "soothe.cognition.agent_loop.completed"
	EventAgentLoopStepStarted   = "soothe.cognition.agent_loop.step.started"
	EventAgentLoopStepCompleted = "soothe.cognition.agent_loop.step.completed"
)

// Branch (retry) events
const (
	EventBranchCreated      = "soothe.cognition.branch.created"
	EventBranchAnalyzed     = "soothe.cognition.branch.analyzed"
	EventBranchRetryStarted = "soothe.cognition.branch.retry.started"
	EventBranchPruned       = "soothe.cognition.branch.pruned"
)

// Message protocol events
const (
	EventMessageReceived = "soothe.protocol.message.received"
	EventMessageSent     = "soothe.protocol.message.sent"
)

// Memory protocol events
const (
	EventMemoryRecalled = "soothe.protocol.memory.recalled"
	EventMemoryStored   = "soothe.protocol.memory.stored"
)

// Policy protocol events
const (
	EventPolicyChecked = "soothe.protocol.policy.checked"
	EventPolicyDenied  = "soothe.protocol.policy.denied"
)

// Output events
const (
	EventFinalReport = "soothe.output.autonomous.final_report.reported"
)

// System events
const (
	EventDaemonHeartbeat = "soothe.system.daemon.heartbeat"
)

// Plugin events
const (
	EventPluginLoaded   = "soothe.plugin.loaded"
	EventPluginFailed   = "soothe.plugin.failed"
	EventPluginUnloaded = "soothe.plugin.unloaded"
)

// Error events
const (
	EventGeneralFailed = "soothe.error.general.failed"
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
		return classifyLifecycleEvent(full)
	case "protocol":
		return TierDetailed
	case "cognition":
		return classifyCognitionEvent(full)
	case "tool":
		return TierInternal
	case "capability":
		// Capability subagents: progress events -> NORMAL, others -> DETAILED
		return classifyCapabilityEvent(full)
	case "output":
		return TierQuiet
	case "system":
		return TierDebug
	case "plugin":
		return TierDetailed
	case "error":
		return TierQuiet
	default:
		return TierNormal
	}
}

func classifyLifecycleEvent(full string) VerbosityTier {
	_, _, action, _ := ParseNamespace(full)
	switch action {
	case "completed", "ended", "error":
		return TierQuiet
	case "started", "reattached":
		return TierNormal
	default:
		return TierDetailed
	}
}

func classifyCognitionEvent(full string) VerbosityTier {
	_, component, _, _ := ParseNamespace(full)
	switch component {
	case "plan", "goal", "agent_loop":
		return TierNormal
	case "branch":
		return TierDetailed
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
	case EventFinalReport,
		EventGeneralFailed, EventGoalFailed, EventPlanStepFailed:
		return TierQuiet
	case EventPlanCreated, EventPlanStepStarted, EventPlanStepCompleted,
		EventPlanBatchStarted, EventPlanReflected, EventPlanDagSnapshot,
		EventGoalCreated, EventGoalCompleted, EventGoalDeferred,
		EventGoalBatchStarted, EventGoalReported, EventGoalDirectivesApplied,
		EventAgentLoopStarted, EventAgentLoopIterated,
		EventAgentLoopStepStarted, EventAgentLoopStepCompleted,
		EventBrowserStarted, EventBrowserCompleted,
		EventClaudeStarted, EventClaudeCompleted,
		EventResearchStarted, EventResearchCompleted,
		EventResearchJudgementReporting:
		return TierNormal
	case EventAgentLoopCompleted:
		return TierQuiet
	case EventIterationStarted, EventIterationCompleted,
		EventCheckpointSaved, EventCheckpointAnchorCreated,
		EventRecoveryResumed, EventBranchCreated, EventBranchAnalyzed,
		EventBranchRetryStarted, EventBranchPruned,
		EventMemoryRecalled, EventMemoryStored,
		EventPolicyChecked, EventPolicyDenied,
		EventLoopCreated, EventLoopStarted, EventLoopDetached,
		EventLoopReattached, EventLoopCompleted, EventLoopHistoryReplayed,
		EventPluginLoaded, EventPluginFailed, EventPluginUnloaded:
		return TierDetailed
	case EventDaemonHeartbeat:
		return TierDebug
	default:
		return TierNormal
	}
}

// IsCompletionEvent checks if an event namespace signals loop/run completion.
func IsCompletionEvent(namespace string) bool {
	_, _, action, ok := ParseNamespace(namespace)
	if !ok {
		return false
	}
	return action == "completed" || namespace == EventLoopCompleted
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
	EventLoopCompleted:              true,
	EventFinalReport:                true,
	EventPlanCreated:                true,
	EventPlanStepStarted:            true,
	EventPlanStepCompleted:          true,
	EventPlanStepFailed:             true,
	EventGoalCreated:                true,
	EventGoalCompleted:              true,
	EventGoalFailed:                 true,
	EventAgentLoopStarted:           true,
	EventAgentLoopIterated:          true,
	EventAgentLoopCompleted:         true,
	EventBrowserStarted:             true,
	EventBrowserCompleted:           true,
	EventClaudeStarted:              true,
	EventClaudeCompleted:            true,
	EventResearchStarted:            true,
	EventResearchCompleted:          true,
	EventResearchJudgementReporting: true,
}
