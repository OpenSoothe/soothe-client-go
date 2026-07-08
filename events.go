package soothe

// Client-facing event namespace constants for the Soothe daemon wire protocol.
// Internal catalog types (soothe.internal.*) are server-only and must not be
// referenced from client libraries.

// Plan events
const (
	EventPlanCreated      = "soothe.cognition.plan.created"
	EventPlanBatchStarted = "soothe.cognition.plan.batch.started"
	EventPlanReflected    = "soothe.cognition.plan.reflected"
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

// Explore subagent events (built-in wire, IG-339)
const (
	EventExploreStarted       = "soothe.subagent.explore.started"
	EventExploreMilestone     = "soothe.subagent.explore.milestone"
	EventExploreStepCompleted = "soothe.subagent.explore.step.completed"
	EventExploreCompleted     = "soothe.subagent.explore.completed"
)

// Tacitus subagent events (built-in wire, IG-339)
const (
	EventTacitusStarted       = "soothe.subagent.tacitus.started"
	EventTacitusGatherSummary = "soothe.subagent.tacitus.gather.summary"
	EventTacitusCompleted     = "soothe.subagent.tacitus.completed"
)

// Control-plane wire envelopes (not soothe.* catalog events)
const (
	EventReplayComplete     = "replay_complete"
	EventLoopReattachedWire = "loop_reattached"
)

// Card ledger replay frames (RFC-413, card_binder design)
const (
	EventCardReplayBegin = "card.replay_begin"
	EventCardCreated     = "card.created"
	EventCardReplayEnd   = "card.replay_end"
)

// Tool events
const (
	EventToolStarted   = "soothe.tool.execution.started"
	EventToolCompleted = "soothe.tool.execution.completed"
	EventToolError     = "soothe.tool.execution.error"
)

// Stream tool call events (RFC-450, IG-416)
const (
	EventStreamToolCallUpdate = "soothe.stream.tool_call.update"
	EventToolCallUpdatesBatch = "tool_call_updates_batch"
)

// StrangeLoop events
const (
	EventStrangeLoopStarted          = "soothe.cognition.strange_loop.started"
	EventStrangeLoopCompleted        = "soothe.cognition.strange_loop.completed"
	EventStrangeLoopPlanDecision     = "soothe.cognition.strange_loop.plan.decision"
	EventStrangeLoopReasoned         = "soothe.cognition.strange_loop.reasoned"
	EventStrangeLoopStepStarted      = "soothe.cognition.strange_loop.step.started"
	EventStrangeLoopStepQueued       = "soothe.cognition.strange_loop.step.queued"
	EventStrangeLoopStepCompleted    = "soothe.cognition.strange_loop.step.completed"
	EventStrangeLoopContextCompacted = "soothe.cognition.strange_loop.context.compacted"
)

// Branch (retry) events — client UX only
const (
	EventBranchCreated      = "soothe.cognition.branch.created"
	EventBranchRetryStarted = "soothe.cognition.branch.retry.started"
)

// Message protocol events (stream metadata)
const (
	EventMessageReceived = "soothe.protocol.message.received"
	EventMessageSent     = "soothe.protocol.message.sent"
)

// Output events
const (
	EventFinalReport = "soothe.output.autonomous.final_report.reported"
)

// Autopilot system events
const (
	EventAutopilotStatusChanged   = "soothe.system.autopilot.status.changed"
	EventAutopilotGoalCreated     = "soothe.system.autopilot.goal.created"
	EventAutopilotGoalProgress    = "soothe.system.autopilot.goal.reported"
	EventAutopilotGoalCompleted   = "soothe.system.autopilot.goal.completed"
	EventAutopilotGoalSuspended   = "soothe.system.autopilot.goal.suspended"
	EventAutopilotGoalBlocked     = "soothe.system.autopilot.goal.blocked"
	EventAutopilotDreamingEntered = "soothe.system.autopilot.dreaming.started"
	EventAutopilotDreamingExited  = "soothe.system.autopilot.dreaming.completed"
)

// Error events
const (
	EventGeneralFailed = "soothe.error.general.failed"
)

// ParseNamespace splits a 4-segment event namespace into its components.
func ParseNamespace(ns string) (domain, component, action string, ok bool) {
	parts := splitNamespace(ns)
	if len(parts) < 4 || parts[0] != "soothe" {
		return "", "", "", false
	}
	if parts[1] == "internal" {
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
