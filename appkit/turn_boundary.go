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
//   - soothe.stream.end (scope=turn) after running + turn progress
//   - status=idle after running + stream payload
//   - status=stopped after running
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
		if strings.EqualFold(state, "stopped") && b.Gate.SawRunning {
			return b.mark(TurnEndStopped)
		}
		if strings.EqualFold(state, "idle") && b.Gate.AllowIdleComplete() {
			return b.mark(TurnEndIdle)
		}
	case soothe.EventMessage:
		if m.Mode == "custom" && soothe.IsTurnEndCustomData(m.Data) && b.Gate.AllowStreamEnd() {
			return b.mark(TurnEndStreamEnd)
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
