package appkit

// TurnEventStats tracks per-turn stream filtering counters.
type TurnEventStats struct {
	FilteredEarly   int
	PostIdleDrained int
	InboundDropped  int
}

// NewTurnEventStats returns an empty stats bag.
func NewTurnEventStats() *TurnEventStats {
	return &TurnEventStats{}
}
