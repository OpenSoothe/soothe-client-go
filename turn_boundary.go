package soothe

import (
	"strconv"
	"strings"
)

// FormatTurnID returns wire turn_id for loopID + admit generation.
func FormatTurnID(loopID string, generation int) string {
	lid := strings.TrimSpace(loopID)
	if lid == "" || generation <= 0 {
		return ""
	}
	return lid + ":" + strconv.Itoa(generation)
}

// ParseTurnGeneration extracts generation int from turn_id, or -1 if malformed.
func ParseTurnGeneration(turnID string) int {
	raw := strings.TrimSpace(turnID)
	if raw == "" || !strings.Contains(raw, ":") {
		return -1
	}
	parts := strings.Split(raw, ":")
	suffix := parts[len(parts)-1]
	gen, err := strconv.Atoi(suffix)
	if err != nil || gen <= 0 {
		return -1
	}
	return gen
}

// FrameTurnID returns turn_id from a status/event frame or nested custom data.
func FrameTurnID(frame map[string]interface{}) string {
	if frame == nil {
		return ""
	}
	if tid, ok := frame["turn_id"].(string); ok {
		if t := strings.TrimSpace(tid); t != "" {
			return t
		}
	}
	if data, ok := frame["data"].(map[string]interface{}); ok {
		if tid, ok := data["turn_id"].(string); ok {
			if t := strings.TrimSpace(tid); t != "" {
				return t
			}
		}
	}
	return ""
}

// FrameSeq returns non-negative seq from a wire frame, or -1.
func FrameSeq(frame map[string]interface{}) int {
	if frame == nil {
		return -1
	}
	switch v := frame["seq"].(type) {
	case int:
		if v >= 0 {
			return v
		}
	case int64:
		if v >= 0 {
			return int(v)
		}
	case float64:
		if v >= 0 && v == float64(int(v)) {
			return int(v)
		}
	}
	return -1
}

// TurnIDsMatch reports whether both ids are non-empty and equal.
// Absent ids never match.
func TurnIDsMatch(expected, candidate string) bool {
	exp := strings.TrimSpace(expected)
	cand := strings.TrimSpace(candidate)
	return exp != "" && cand != "" && exp == cand
}

// IsTurnTerminalAllowed gates turn-scoped stream.end / strange_loop.completed.
func IsTurnTerminalAllowed(expectedTurnID, frameTurnID string, queryStarted, turnProgressSeen bool) bool {
	if !queryStarted || !turnProgressSeen {
		return false
	}
	return TurnIDsMatch(expectedTurnID, frameTurnID)
}

// IsIdleTerminalAllowed gates status=idle soft-complete.
func IsIdleTerminalAllowed(
	expectedTurnID, frameTurnID string,
	queryStarted, turnProgressSeen, cancellationSeen bool,
) bool {
	if !queryStarted || strings.TrimSpace(expectedTurnID) == "" {
		return false
	}
	cand := strings.TrimSpace(frameTurnID)
	if cand != "" {
		if !TurnIDsMatch(expectedTurnID, cand) {
			return false
		}
		return turnProgressSeen || cancellationSeen
	}
	return cancellationSeen
}
