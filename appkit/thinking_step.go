package appkit

import (
	"fmt"
	"strings"

	soothe "github.com/mirasoth/soothe-client-go"
)

// eventLoopHistoryReplayed is the daemon's replay completion signal
// (internal; not exported by soothe-client-go).
const eventLoopHistoryReplayed = "soothe.lifecycle.loop.history.replayed"

const maxThinkingStepRunes = 280

// Default thinking-step event allowlist (triarch's set). Overridable via
// ClassifierConfig.ThinkingStepEvents.
var defaultThinkingStepEvents = map[string]bool{
	"soothe.cognition.plan.step.started":   true,
	"soothe.cognition.plan.step.completed": true,
	"soothe.cognition.plan.step.failed":    true,
	"soothe.lifecycle.iteration.started":   true,
	"soothe.agent.loop.step.started":       true,
	"soothe.agent.loop.started":            true,
	soothe.EventPlanBatchStarted:           true,
	soothe.EventPlanCreated:                true,
	soothe.EventGoalCreated:                true,
	soothe.EventToolStarted:                true,
}

// extractThinkingStep maps an allowlisted progress event to one structured UI
// line. Free-form streams (tokens, reports, reasoning) are excluded. Ported
// from triarch's thinking_step.go with the allowlist made configurable.
func (cl *EventClassifier) extractThinkingStep(eventType string, data map[string]interface{}) (string, bool) {
	if eventType == "" || data == nil {
		return "", false
	}
	eventType = strings.TrimSpace(eventType)

	allow := cl.cfg.ThinkingStepEvents
	if allow == nil {
		allow = defaultThinkingStepEvents
	}
	if !allow[eventType] {
		return "", false
	}

	var line string
	switch eventType {
	case "soothe.cognition.plan.step.started":
		line = formatPlanStepLine(data, "")
	case "soothe.cognition.plan.step.completed":
		line = formatPlanStepLine(data, "done")
	case "soothe.cognition.plan.step.failed":
		stepID := strField(data, "step_id")
		errMsg := strField(data, "error")
		switch {
		case stepID != "" && errMsg != "":
			line = fmt.Sprintf("Step %s failed: %s", stepID, errMsg)
		case stepID != "":
			line = fmt.Sprintf("Step %s failed", stepID)
		case errMsg != "":
			line = fmt.Sprintf("Step failed: %s", errMsg)
		}
	case "soothe.agent.loop.step.started":
		line = formatAgentStepLine(data, "")
	case soothe.EventPlanBatchStarted:
		if n, ok := data["parallel_count"].(float64); ok && n > 0 {
			line = fmt.Sprintf("Running %d steps in parallel", int(n))
		}
	case soothe.EventPlanCreated, "soothe.agent.loop.started":
		if g := strField(data, "goal"); g != "" {
			line = "Goal: " + g
		}
	case soothe.EventGoalCreated:
		if g := strField(data, "friendly_message", "description"); g != "" {
			line = "Goal: " + g
		}
	case "soothe.lifecycle.iteration.started":
		if g := strField(data, "goal_description"); g != "" {
			line = "Iteration: " + g
		}
	case soothe.EventToolStarted:
		if name := strField(data, "tool_name", "name"); name != "" {
			line = "Tool: " + name
		}
	default:
		return "", false
	}

	line = strings.TrimSpace(line)
	if line == "" {
		return "", false
	}
	if len([]rune(line)) > maxThinkingStepRunes {
		runes := []rune(line)
		line = string(runes[:maxThinkingStepRunes]) + "…"
	}
	return line, true
}

func formatPlanStepLine(data map[string]interface{}, suffix string) string {
	stepID := strField(data, "step_id")
	desc := strField(data, "description")
	switch {
	case stepID != "" && suffix != "":
		return fmt.Sprintf("Step %s: %s", stepID, suffix)
	case stepID != "" && desc != "":
		return fmt.Sprintf("Step %s: %s", stepID, desc)
	case stepID != "":
		return fmt.Sprintf("Step %s", stepID)
	case desc != "" && suffix != "":
		return fmt.Sprintf("Step: %s", suffix)
	case desc != "":
		return fmt.Sprintf("Step: %s", desc)
	case suffix != "":
		return "Step: " + suffix
	default:
		return ""
	}
}

func formatAgentStepLine(data map[string]interface{}, suffix string) string {
	stepID := strField(data, "step_id")
	desc := strField(data, "description")
	switch {
	case stepID != "" && desc != "":
		return fmt.Sprintf("Step %s: %s", stepID, desc)
	case desc != "":
		if suffix != "" {
			return fmt.Sprintf("Step: %s", suffix)
		}
		return fmt.Sprintf("Step: %s", desc)
	case stepID != "":
		return fmt.Sprintf("Step %s", stepID)
	default:
		return ""
	}
}

func strField(data map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := data[key].(string); ok {
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		}
	}
	return ""
}

func thinkingStepResult(step string) ChatEventResult {
	return ChatEventResult{
		ThinkingStep: strings.TrimSpace(step),
		Terminal:     ChatEventContinue,
	}
}
