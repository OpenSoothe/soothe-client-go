package appkit

import (
	"strings"

	soothe "github.com/mirasoth/soothe-client-go"
)

// Turn end reasons aligned with DaemonSession.IterTurnChunks (wire contract).
const (
	TurnEndStreamEnd = soothe.STREAM_END
	TurnEndIdle      = "status.idle"
	TurnEndStopped   = "status.stopped"
)

// TurnBoundary applies DaemonSession turn-end rules to decoded pool messages.
// TurnRunner owns one per Execute; EventClassifier must not be the authority
// for turn termination (phases may early-complete for UX only).
//
// Contract (soothe-cli / Python DaemonSession parity):
//   - soothe.stream.end (scope=turn) after bound turn_id + running + turn progress
//   - status=idle after bound matching turn_id + progress (or cancel)
//   - status=stopped after running (+ matching turn_id when bound)
type TurnBoundary struct {
	Gate   TurnLifecycleGate
	Ended  bool
	Reason string
}

// Feed observes msg and reports whether the daemon turn has ended.
func (b *TurnBoundary) Feed(msg interface{}) (ended bool, reason string) {
	if b == nil {
		return false, ""
	}
	if b.Ended {
		return true, b.Reason
	}
	b.Gate.Observe(msg)

	switch m := msg.(type) {
	case soothe.StatusResponse:
		state := strings.TrimSpace(m.State)
		frameTurn := strings.TrimSpace(m.TurnID)
		if strings.EqualFold(state, "stopped") && b.Gate.SawRunning {
			if b.Gate.ExpectedTurnID != "" && !soothe.TurnIDsMatch(b.Gate.ExpectedTurnID, frameTurn) {
				return false, ""
			}
			return b.mark(TurnEndStopped)
		}
		if strings.EqualFold(state, "idle") && b.Gate.AllowIdleComplete(frameTurn) {
			return b.mark(TurnEndIdle)
		}
	case soothe.EventMessage:
		frameTurn := strings.TrimSpace(m.TurnID)
		if data, ok := m.Data.(map[string]interface{}); ok {
			if tid := soothe.FrameTurnID(data); tid != "" {
				frameTurn = tid
			}
		}
		if m.Mode == "custom" && soothe.IsTurnEndCustomData(m.Data) && b.Gate.AllowStreamEnd(frameTurn) {
			return b.mark(TurnEndStreamEnd)
		}
	case map[string]interface{}:
		typ := asString(m["type"])
		if typ == "status" {
			state := asString(m["state"])
			frameTurn := soothe.FrameTurnID(m)
			if state == "stopped" && b.Gate.SawRunning {
				if b.Gate.ExpectedTurnID != "" && !soothe.TurnIDsMatch(b.Gate.ExpectedTurnID, frameTurn) {
					return false, ""
				}
				return b.mark(TurnEndStopped)
			}
			if state == "idle" && b.Gate.AllowIdleComplete(frameTurn) {
				return b.mark(TurnEndIdle)
			}
		}
		if typ == "event" || asString(m["mode"]) != "" {
			mode := asString(m["mode"])
			data := m["data"]
			frameTurn := soothe.FrameTurnID(m)
			if dm, ok := data.(map[string]interface{}); ok {
				if tid := soothe.FrameTurnID(dm); tid != "" {
					frameTurn = tid
				}
			}
			if mode == "custom" && soothe.IsTurnEndCustomData(data) && b.Gate.AllowStreamEnd(frameTurn) {
				return b.mark(TurnEndStreamEnd)
			}
		}
	}
	return false, ""
}

func (b *TurnBoundary) mark(reason string) (bool, string) {
	b.Ended = true
	b.Reason = reason
	return true, reason
}

// IsDaemonTurnEndEvent reports completion_event values produced by TurnBoundary
// (not phase-tagged deliverables).
func IsDaemonTurnEndEvent(completionEvent string) bool {
	switch strings.TrimSpace(completionEvent) {
	case TurnEndStreamEnd, TurnEndIdle, TurnEndStopped:
		return true
	default:
		return false
	}
}
