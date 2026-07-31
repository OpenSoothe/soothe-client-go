package appkit

import (
	"strings"

	soothe "github.com/mirasoth/soothe-client-go"
)

// TurnLifecycleGate tracks DaemonSession-equivalent progress for a single
// TurnRunner turn. It is owned by the turn loop (not shared across chats) so a
// pooled EventClassifier stays race-free under concurrent Execute calls.
//
// Mirrors client/python DaemonSession.iter_turn_chunks:
//   - stream.end requires bound turn_id + running + turn progress
//   - status=idle requires bound matching turn_id + progress (or cancel)
type TurnLifecycleGate struct {
	SawRunning       bool
	SawTurnProgress  bool
	ExpectedTurnID   string
	CancellationSeen bool
}

// Observe updates gate flags from one decoded inbound message.
func (g *TurnLifecycleGate) Observe(msg interface{}) {
	if g == nil {
		return
	}
	switch m := msg.(type) {
	case soothe.StatusResponse:
		if strings.EqualFold(strings.TrimSpace(m.State), "running") {
			g.SawRunning = true
			if tid := strings.TrimSpace(m.TurnID); tid != "" {
				newGen := soothe.ParseTurnGeneration(tid)
				oldGen := soothe.ParseTurnGeneration(g.ExpectedTurnID)
				if g.ExpectedTurnID == "" || (newGen > 0 && (oldGen < 0 || newGen >= oldGen)) {
					if g.ExpectedTurnID != "" && tid != g.ExpectedTurnID {
						g.SawTurnProgress = false
					}
					g.ExpectedTurnID = tid
				}
			}
		}
	case soothe.EventMessage:
		if soothe.IsTurnProgressChunk(m.Mode, m.Data) {
			g.SawTurnProgress = true
		}
	case map[string]interface{}:
		typ := asString(m["type"])
		if typ == "status" && asString(m["state"]) == "running" {
			g.SawRunning = true
			if tid := soothe.FrameTurnID(m); tid != "" {
				newGen := soothe.ParseTurnGeneration(tid)
				oldGen := soothe.ParseTurnGeneration(g.ExpectedTurnID)
				if g.ExpectedTurnID == "" || (newGen > 0 && (oldGen < 0 || newGen >= oldGen)) {
					if g.ExpectedTurnID != "" && tid != g.ExpectedTurnID {
						g.SawTurnProgress = false
					}
					g.ExpectedTurnID = tid
				}
			}
		}
		if typ == "event" || asString(m["mode"]) != "" {
			mode := asString(m["mode"])
			if soothe.IsTurnProgressChunk(mode, m["data"]) {
				g.SawTurnProgress = true
			}
		}
	}
}

// AllowStreamEnd reports whether a turn-scoped soothe.stream.end may end the turn.
func (g *TurnLifecycleGate) AllowStreamEnd(frameTurnID string) bool {
	if g == nil {
		return false
	}
	return soothe.IsTurnTerminalAllowed(g.ExpectedTurnID, frameTurnID, g.SawRunning, g.SawTurnProgress)
}

// AllowIdleComplete reports whether status=idle may soft-complete the turn.
func (g *TurnLifecycleGate) AllowIdleComplete(frameTurnID string) bool {
	if g == nil {
		return false
	}
	return soothe.IsIdleTerminalAllowed(
		g.ExpectedTurnID, frameTurnID, g.SawRunning, g.SawTurnProgress, g.CancellationSeen,
	)
}
