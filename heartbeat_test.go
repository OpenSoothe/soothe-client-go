package soothe

import (
	"fmt"
	"testing"
	"time"
)

func TestHeartbeatTracker_New(t *testing.T) {
	tracker := NewHeartbeatTracker()
	if tracker == nil {
		t.Fatal("expected tracker to be created")
	}

	// Initial state should be empty
	health := tracker.GetHealth()
	if health.State != "" {
		t.Errorf("expected empty initial state, got %q", health.State)
	}
	if health.LoopID != "" {
		t.Errorf("expected empty initial loop ID, got %q", health.LoopID)
	}

	// Should be alive during grace period (first 20 seconds)
	if !health.IsAlive {
		t.Error("expected daemon to be alive during grace period")
	}
}

func TestHeartbeatTracker_NewWithThreshold(t *testing.T) {
	cfg := GetCIConfig()

	// Use a threshold that gives enough buffer for the test
	customThreshold := cfg.HeartbeatThreshold
	tracker := NewHeartbeatTrackerWithThreshold(customThreshold)
	if tracker == nil {
		t.Fatal("expected tracker to be created")
	}

	// Check that custom threshold is applied
	tracker.Update(map[string]interface{}{
		"state":   "running",
		"loop_id": "loop-123",
	})

	// Should still be alive right after update
	health := tracker.GetHealth()
	if !health.IsAlive {
		t.Error("expected daemon to be alive immediately after heartbeat")
	}

	// Simulate both grace period and heartbeat threshold being exceeded
	// The grace period (20s) and threshold need to both be exceeded for daemon to be not alive
	tracker.mu.Lock()
	tracker.startTime = time.Now().Add(-25 * time.Second)                          // Exceed 20s grace period
	tracker.lastHeartbeat = time.Now().Add(-customThreshold - 50*time.Millisecond) // Exceed threshold
	tracker.mu.Unlock()

	// Small additional wait to ensure threshold exceeded
	time.Sleep(10 * time.Millisecond)

	health = tracker.GetHealth()
	if health.IsAlive {
		t.Errorf("expected daemon to not be alive after both grace period (20s) and threshold (%v) exceeded, CI=%v", customThreshold, cfg.CI)
	}
}

func TestHeartbeatTracker_Update(t *testing.T) {
	tracker := NewHeartbeatTracker()

	heartbeatData := map[string]interface{}{
		"state":     "running",
		"loop_id":   "loop-456",
		"timestamp": "2024-01-01T00:00:00Z",
	}

	tracker.Update(heartbeatData)

	health := tracker.GetHealth()
	if health.State != "running" {
		t.Errorf("expected state 'running', got %q", health.State)
	}
	if health.LoopID != "loop-456" {
		t.Errorf("expected loop_id 'loop-456', got %q", health.LoopID)
	}
	if health.LastHeartbeat.IsZero() {
		t.Error("expected last heartbeat timestamp to be set")
	}
	if !health.IsAlive {
		t.Error("expected daemon to be alive after heartbeat")
	}
}

func TestHeartbeatTracker_StateMethods(t *testing.T) {
	tracker := NewHeartbeatTracker()

	// Test idle state
	tracker.Update(map[string]interface{}{
		"state":   "idle",
		"loop_id": "",
	})

	if !tracker.IsIdle() {
		t.Error("expected IsIdle to return true")
	}
	if tracker.IsProcessing() {
		t.Error("expected IsProcessing to return false")
	}
	if tracker.GetState() != "idle" {
		t.Errorf("expected GetState to return 'idle', got %q", tracker.GetState())
	}

	// Test running state
	tracker.Update(map[string]interface{}{
		"state":   "running",
		"loop_id": "loop-789",
	})

	if tracker.IsIdle() {
		t.Error("expected IsIdle to return false")
	}
	if !tracker.IsProcessing() {
		t.Error("expected IsProcessing to return true")
	}
	if tracker.GetState() != "running" {
		t.Errorf("expected GetState to return 'running', got %q", tracker.GetState())
	}
	if tracker.GetLoopID() != "loop-789" {
		t.Errorf("expected GetLoopID to return 'loop-789', got %q", tracker.GetLoopID())
	}
}

func TestHeartbeatTracker_ProcessHeartbeatEvent(t *testing.T) {
	tracker := NewHeartbeatTracker()

	// Test non-event message
	regularMsg := map[string]interface{}{
		"type":    "status",
		"loop_id": "loop-123",
	}
	if tracker.ProcessHeartbeatEvent(regularMsg) {
		t.Error("expected ProcessHeartbeatEvent to return false for non-event message")
	}

	// Catalog heartbeat-shaped events are not delivered to clients.
	heartbeatEvent := map[string]interface{}{
		"type": "event",
		"data": map[string]interface{}{
			"type": "soothe.internal.daemon.heartbeat",
		},
	}
	if tracker.ProcessHeartbeatEvent(heartbeatEvent) {
		t.Error("expected ProcessHeartbeatEvent to ignore server-internal heartbeat events")
	}

	health := tracker.GetHealth()
	if health.State != "" {
		t.Errorf("expected empty state when heartbeats are not on the wire, got %q", health.State)
	}
}

func TestHeartbeatTracker_Reset(t *testing.T) {
	tracker := NewHeartbeatTracker()

	// Update with some data
	tracker.Update(map[string]interface{}{
		"state":   "running",
		"loop_id": "loop-123",
	})

	// Reset
	tracker.Reset()

	health := tracker.GetHealth()
	if health.State != "" {
		t.Errorf("expected empty state after reset, got %q", health.State)
	}
	if health.LoopID != "" {
		t.Errorf("expected empty loop_id after reset, got %q", health.LoopID)
	}
	// Should be alive again during grace period
	if !health.IsAlive {
		t.Error("expected daemon to be alive during grace period after reset")
	}
}

func TestHeartbeatTracker_SetAliveThreshold(t *testing.T) {
	tracker := NewHeartbeatTracker()

	// Set custom threshold
	tracker.SetAliveThreshold(5 * time.Second)

	// Update heartbeat
	tracker.Update(map[string]interface{}{
		"state": "running",
	})

	// Should be alive immediately after update
	if !tracker.GetHealth().IsAlive {
		t.Error("expected daemon to be alive immediately after heartbeat")
	}

	// Wait 6 seconds (over threshold)
	// Simulate by adjusting startTime to bypass grace period
	tracker.mu.Lock()
	tracker.startTime = time.Now().Add(-25 * time.Second)
	tracker.mu.Unlock()

	// Manually set last heartbeat to 6 seconds ago
	tracker.mu.Lock()
	tracker.lastHeartbeat = time.Now().Add(-6 * time.Second)
	tracker.mu.Unlock()

	// Should no longer be alive
	if tracker.GetHealth().IsAlive {
		t.Error("expected daemon to not be alive after threshold exceeded")
	}
}

func TestHeartbeatTracker_ConcurrentAccess(t *testing.T) {
	tracker := NewHeartbeatTracker()

	// Concurrent updates
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			tracker.Update(map[string]interface{}{
				"state":   "running",
				"loop_id": fmt.Sprintf("loop-%d", id),
			})
			_ = tracker.GetHealth()
			_ = tracker.GetHealth().IsAlive
			_ = tracker.IsProcessing()
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Tracker should still be in a valid state
	health := tracker.GetHealth()
	if health.State == "" {
		t.Error("expected non-empty state after concurrent updates")
	}
}

func TestHeartbeatTracker_GetLastHeartbeat(t *testing.T) {
	tracker := NewHeartbeatTracker()

	// Initially should be zero
	if !tracker.GetLastHeartbeat().IsZero() {
		t.Error("expected initial last heartbeat to be zero")
	}

	// Update heartbeat
	before := time.Now()
	tracker.Update(map[string]interface{}{
		"state": "running",
	})
	after := time.Now()

	lastHB := tracker.GetLastHeartbeat()
	if lastHB.IsZero() {
		t.Error("expected last heartbeat to be set")
	}
	if lastHB.Before(before) || lastHB.After(after) {
		t.Errorf("last heartbeat %v not in expected range [%v, %v]", lastHB, before, after)
	}
}

func TestHeartbeatError(t *testing.T) {
	err := &HeartbeatError{
		LastHeartbeat: time.Now(),
		State:         "idle",
		Message:       "daemon timeout",
	}

	errStr := err.Error()
	if errStr == "" {
		t.Error("expected non-empty error string")
	}

	// Test with zero heartbeat
	err2 := &HeartbeatError{
		LastHeartbeat: time.Time{},
		State:         "",
		Message:       "daemon timeout",
	}

	errStr2 := err2.Error()
	if errStr2 == "" {
		t.Error("expected non-empty error string for zero heartbeat")
	}
}
