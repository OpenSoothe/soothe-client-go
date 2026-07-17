package soothe

// Inbound drop priorities (lower = keep, higher = drop candidate). Matches Python.
const (
	DropPriorityCritical = 0 // Never drop: terminals, status, errors
	DropPriorityHigh     = 1 // Prefer keep: cognition / tool batches
	DropPriorityNormal   = 2 // Default drop candidate: streaming chunks
)

// DefaultInboundMaxSize is the pending-event cap (Python inbound queue parity).
const DefaultInboundMaxSize = 20_000

// InboundFrameDropPriority returns drop priority for an inbound frame.
func InboundFrameDropPriority(event map[string]interface{}) int {
	if event == nil {
		return DropPriorityCritical
	}
	eventType := asString(event["type"])
	if eventType == "event_batch" || eventType == "tool_call_updates_batch" {
		return DropPriorityHigh
	}
	if eventType == "next" {
		payload, _ := event["payload"].(map[string]interface{})
		if payload != nil {
			innerMode := asString(payload["mode"])
			innerData := payload["data"]
			if innerMode == "messages" {
				if messagesWireTerminal(innerData) {
					return DropPriorityCritical
				}
				if arr, ok := innerData.([]interface{}); ok && len(arr) > 0 {
					if first, ok := arr[0].(map[string]interface{}); ok {
						if asString(first["phase"]) == "goal_completion" {
							return DropPriorityCritical
						}
					}
				}
			}
			if asString(payload["type"]) == "complete" {
				return DropPriorityCritical
			}
			if inner, ok := innerData.(map[string]interface{}); ok {
				return InboundFrameDropPriority(inner)
			}
			eventType = asString(payload["type"])
		}
	}
	if eventType == "complete" || eventType == "error" || eventType == "connection_ack" {
		return DropPriorityCritical
	}
	if eventType == "status" {
		state := asString(event["state"])
		switch state {
		case "idle", "running", "stopped", "detached":
			return DropPriorityCritical
		}
	}
	if eventType == "event" {
		mode := asString(event["mode"])
		data := event["data"]
		if mode == "custom" {
			if IsTurnEndCustomData(data) {
				return DropPriorityCritical
			}
			if m, ok := data.(map[string]interface{}); ok {
				customType := asString(m["type"])
				if hasPrefix(customType, "soothe.cognition.") {
					return DropPriorityHigh
				}
				if hasPrefix(customType, "soothe.error.") || customType == "stream_degraded" {
					return DropPriorityCritical
				}
				if customType == "soothe.ux.stream_tool_wire.tool_call_updates_batch" {
					return DropPriorityHigh
				}
			}
		}
		if mode == "messages" {
			if messagesWireTerminal(data) {
				return DropPriorityCritical
			}
			if arr, ok := data.([]interface{}); ok && len(arr) > 0 {
				if first, ok := arr[0].(map[string]interface{}); ok {
					if asString(first["phase"]) == "goal_completion" {
						return DropPriorityCritical
					}
				}
			}
		}
	}
	return DropPriorityNormal
}

func messagesWireTerminal(data interface{}) bool {
	arr, ok := data.([]interface{})
	if !ok || len(arr) == 0 {
		return false
	}
	body, _ := arr[0].(map[string]interface{})
	if body == nil {
		return false
	}
	t := asString(body["type"])
	return t == STREAM_END || containsStr(t, "stream.end")
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func containsStr(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
