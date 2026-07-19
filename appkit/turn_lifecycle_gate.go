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
//   - stream.end requires saw running + turn progress
//   - status=idle requires saw running + any stream payload
type TurnLifecycleGate struct {
	SawRunning       bool
	SawStreamPayload bool
	SawTurnProgress  bool
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
		}
	case soothe.EventMessage:
		g.SawStreamPayload = true
		if soothe.IsTurnProgressChunk(m.Mode, m.Data) {
			g.SawTurnProgress = true
		}
	}
}

// AllowStreamEnd reports whether a turn-scoped soothe.stream.end may end the turn.
func (g *TurnLifecycleGate) AllowStreamEnd() bool {
	return g != nil && g.SawRunning && g.SawTurnProgress
}

// AllowIdleComplete reports whether status=idle may soft-complete the turn.
func (g *TurnLifecycleGate) AllowIdleComplete() bool {
	return g != nil && g.SawRunning && g.SawStreamPayload
}
