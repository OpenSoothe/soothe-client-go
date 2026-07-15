package soothe

import (
	"sync"
	"time"
)

// DaemonHealth represents the daemon's health status based on heartbeat tracking.
type DaemonHealth struct {
	LastHeartbeat time.Time // Timestamp of last received heartbeat
	State         string    // Daemon state: "running" or "idle"
	LoopID        string    // Loop ID the daemon is currently processing (if running)
	IsAlive       bool      // True if heartbeat received within the alive threshold
}

// HeartbeatTracker tracks daemon heartbeat events to monitor daemon aliveness.
// It is safe for concurrent use from multiple goroutines.
type HeartbeatTracker struct {
	mu              sync.RWMutex
	lastHeartbeat   time.Time
	daemonState     string
	heartbeatLoopID string
	aliveThreshold  time.Duration // Max time since last heartbeat to consider daemon alive
	startTime       time.Time     // When tracking started (for initial grace period)
}

// NewHeartbeatTracker creates a new heartbeat tracker with default settings.
// Default alive threshold is 15 seconds (allows 2 missed heartbeats at 5s interval).
func NewHeartbeatTracker() *HeartbeatTracker {
	return &HeartbeatTracker{
		aliveThreshold: 15 * time.Second,
		startTime:      time.Now(),
	}
}

// NewHeartbeatTrackerWithThreshold creates a tracker with a custom alive threshold.
func NewHeartbeatTrackerWithThreshold(aliveThreshold time.Duration) *HeartbeatTracker {
	return &HeartbeatTracker{
		aliveThreshold: aliveThreshold,
		startTime:      time.Now(),
	}
}

// Update processes a heartbeat event and updates the tracker's state.
// Pass the heartbeat event data map (from event["data"]).
func (t *HeartbeatTracker) Update(heartbeatData map[string]interface{}) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.lastHeartbeat = time.Now()

	// Extract daemon state
	if state, ok := heartbeatData["state"].(string); ok {
		t.daemonState = state
	}

	if loopID, ok := heartbeatData["loop_id"].(string); ok {
		t.heartbeatLoopID = loopID
	}
}

// GetHealth returns the current daemon health status.
func (t *HeartbeatTracker) GetHealth() *DaemonHealth {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Consider daemon alive if:
	// 1. We've received a heartbeat within the alive threshold
	// 2. OR we're in the initial grace period (first 20 seconds after tracking started)
	//    This handles cold-start scenarios where daemon may not be sending heartbeats yet
	timeSinceLast := time.Since(t.lastHeartbeat)
	timeSinceStart := time.Since(t.startTime)
	gracePeriod := 20 * time.Second

	isAlive := timeSinceLast < t.aliveThreshold || timeSinceStart < gracePeriod

	return &DaemonHealth{
		LastHeartbeat: t.lastHeartbeat,
		State:         t.daemonState,
		LoopID:        t.heartbeatLoopID,
		IsAlive:       isAlive,
	}
}

// IsProcessing returns true if the daemon is actively processing a query (state == "running").
func (t *HeartbeatTracker) IsProcessing() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.daemonState == "running"
}

// IsIdle returns true if the daemon is idle (state == "idle").
func (t *HeartbeatTracker) IsIdle() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.daemonState == "idle"
}

// GetState returns the current daemon state.
func (t *HeartbeatTracker) GetState() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.daemonState
}

// GetLoopID returns the loop ID the daemon is currently processing.
func (t *HeartbeatTracker) GetLoopID() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.heartbeatLoopID
}

// GetLastHeartbeat returns the timestamp of the last received heartbeat.
func (t *HeartbeatTracker) GetLastHeartbeat() time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lastHeartbeat
}

// SetAliveThreshold updates the alive threshold duration.
func (t *HeartbeatTracker) SetAliveThreshold(threshold time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.aliveThreshold = threshold
}

// Reset clears the tracker state and resets the start time.
// Useful when reconnecting to the daemon.
func (t *HeartbeatTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastHeartbeat = time.Time{}
	t.daemonState = ""
	t.heartbeatLoopID = ""
	t.startTime = time.Now()
}

// ProcessHeartbeatEvent processes a full event message and extracts heartbeat data.
// Returns true if the event was a heartbeat and was processed.
func (t *HeartbeatTracker) ProcessHeartbeatEvent(event map[string]interface{}) bool {
	// Check if this is an event message
	eventType, ok := event["type"].(string)
	if !ok || eventType != "event" {
		return false
	}

	// Extract data field
	data, ok := event["data"].(map[string]interface{})
	if !ok {
		return false
	}

	// Catalog daemon heartbeats (soothe.internal.*) are server-only and are not
	// broadcast to WebSocket clients.
	_ = data
	return false
}

// WaitForAlive blocks until the daemon is considered alive or timeout is reached.
// Returns nil if daemon is alive, error otherwise.
func (t *HeartbeatTracker) WaitForAlive(timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	checkInterval := time.NewTimer(1 * time.Second)
	defer checkInterval.Stop()

	for {
		select {
		case <-deadline.C:
			health := t.GetHealth()
			return &HeartbeatError{
				LastHeartbeat: health.LastHeartbeat,
				State:         health.State,
				Message:       "timeout waiting for daemon to be alive",
			}
		case <-checkInterval.C:
			if t.GetHealth().IsAlive {
				return nil
			}
			checkInterval.Reset(1 * time.Second)
		}
	}
}

// HeartbeatError represents an error related to daemon heartbeat tracking.
type HeartbeatError struct {
	LastHeartbeat time.Time
	State         string
	Message       string
}

func (e *HeartbeatError) Error() string {
	if e.LastHeartbeat.IsZero() {
		return e.Message + " (no heartbeat received)"
	}
	return e.Message + " (last heartbeat: " + e.LastHeartbeat.Format(time.RFC3339) + ", state: " + e.State + ")"
}
