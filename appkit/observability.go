package appkit

// TurnEventStats tracks per-turn stream filtering counters.
type TurnEventStats struct {
	Total             int
	Messages          int
	Updates           int
	Custom            int
	Skipped           int
	FilteredEarly     int
	ToolCalls         int
	ToolResults       int
	TextChunks        int
	HeartbeatsDropped int
	PostIdleDrained   int
	InboundDropped    int
}

// NewTurnEventStats returns an empty stats bag.
func NewTurnEventStats() *TurnEventStats {
	return &TurnEventStats{}
}
