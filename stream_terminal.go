package soothe

import "strings"

// STREAM_END is the daemon turn-scoped stream end custom type.
const STREAM_END = "soothe.stream.end"

// turnEndCustomTypes are custom event types that end a turn.
var turnEndCustomTypes = map[string]struct{}{
	STREAM_END:                {},
	EventStrangeLoopCompleted: {},
}

// turnProgressCustomTypes prove non-intake progress for the active turn.
var turnProgressCustomTypes = map[string]struct{}{
	EventPlanCreated:              {},
	EventStrangeLoopStepStarted:   {},
	EventStrangeLoopStepQueued:    {},
	EventStrangeLoopStepCompleted: {},
	EventCardCreated:              {},
	EventCardUpdated:              {},
	EventCardFinalized:            {},
}

// staleTurnPendingTypes are handshake / card-replay leftovers safe to drop at turn start.
var staleTurnPendingTypes = map[string]struct{}{
	"connection_ack":     {},
	EventCardReplayBegin: {},
	EventCardReplayEnd:   {},
	EventCardCreated:     {},
	"complete":           {},
}

// IsTurnEndCustomData reports whether data is a turn-scoped terminal custom payload.
func IsTurnEndCustomData(data interface{}) bool {
	m, ok := data.(map[string]interface{})
	if !ok || m == nil {
		return false
	}
	customType := strings.TrimSpace(asString(m["type"]))
	if _, ok := turnEndCustomTypes[customType]; !ok {
		return false
	}
	if customType == STREAM_END {
		scope := strings.ToLower(strings.TrimSpace(asString(m["scope"])))
		if scope == "" {
			scope = "turn"
		}
		return scope == "turn"
	}
	return true
}

// IsTurnProgressChunk reports whether a chunk proves non-intake turn progress.
func IsTurnProgressChunk(mode string, data interface{}) bool {
	if mode == "messages" || mode == "updates" {
		return true
	}
	if mode != "custom" {
		return false
	}
	if IsTurnEndCustomData(data) {
		return false
	}
	m, ok := data.(map[string]interface{})
	if !ok || m == nil {
		return false
	}
	customType := strings.TrimSpace(asString(m["type"]))
	if _, ok := turnProgressCustomTypes[customType]; ok {
		return true
	}
	return strings.HasPrefix(customType, "soothe.cognition.strange_loop.step")
}

// StalePendingFrameLabel returns a peel label when event is safe to drop at turn start.
func StalePendingFrameLabel(event map[string]interface{}) string {
	if event == nil {
		return ""
	}
	eventType := asString(event["type"])
	if _, ok := staleTurnPendingTypes[eventType]; ok {
		return eventType
	}
	if eventType == "next" {
		payload, _ := event["payload"].(map[string]interface{})
		if payload == nil {
			return ""
		}
		staleMode := asString(payload["mode"])
		if _, ok := staleTurnPendingTypes[staleMode]; ok {
			return staleMode
		}
		if inner, ok := payload["data"].(map[string]interface{}); ok {
			return StalePendingFrameLabel(inner)
		}
		return ""
	}
	if eventType == "event" {
		mode := asString(event["mode"])
		if mode == "custom" && IsTurnEndCustomData(event["data"]) {
			if m, ok := event["data"].(map[string]interface{}); ok {
				return strings.TrimSpace(asString(m["type"]))
			}
		}
	}
	return ""
}

// InboundNeedsDeliveryAck reports whether the client should bump delivery ack for event.
func InboundNeedsDeliveryAck(event map[string]interface{}) bool {
	if event == nil {
		return false
	}
	switch asString(event["type"]) {
	case "complete":
		return true
	case "next":
		payload, _ := event["payload"].(map[string]interface{})
		if payload == nil {
			return false
		}
		inner, _ := payload["data"].(map[string]interface{})
		if inner == nil {
			return false
		}
		mode := asString(payload["mode"])
		if mode == "event" {
			return inboundNeedsAckFromEventShape(inner)
		}
		return false
	case "event":
		return inboundNeedsAckFromEventShape(event)
	}
	return false
}

func inboundNeedsAckFromEventShape(event map[string]interface{}) bool {
	mode := asString(event["mode"])
	data := event["data"]
	if mode == "custom" && IsTurnEndCustomData(data) {
		return true
	}
	if mode == "messages" {
		arr, ok := data.([]interface{})
		if !ok || len(arr) == 0 {
			return false
		}
		body, _ := arr[0].(map[string]interface{})
		if body == nil {
			return false
		}
		// Terminal wire dicts use stream-end style markers in type/phase.
		t := asString(body["type"])
		return t == STREAM_END || strings.Contains(t, "stream.end")
	}
	return false
}

// ExtractLoopIDFromInbound finds loop_id on a frame or nested next payload.
func ExtractLoopIDFromInbound(event map[string]interface{}) string {
	if event == nil {
		return ""
	}
	if id := strings.TrimSpace(asString(event["loop_id"])); id != "" {
		return id
	}
	if asString(event["type"]) == "next" {
		payload, _ := event["payload"].(map[string]interface{})
		if payload == nil {
			return ""
		}
		if id := strings.TrimSpace(asString(payload["loop_id"])); id != "" {
			return id
		}
		if inner, ok := payload["data"].(map[string]interface{}); ok {
			return strings.TrimSpace(asString(inner["loop_id"]))
		}
	}
	return ""
}
