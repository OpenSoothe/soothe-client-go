package soothe_test

import (
	"context"
	"fmt"
	"time"

	soothe "github.com/mirasoth/soothe-client-go"
)

// Example_heartbeatTracking demonstrates how to use heartbeat tracking
// to monitor daemon health and prevent timeouts during long operations.
func Example_heartbeatTracking() {
	// Create client with heartbeat tracking enabled
	client := soothe.NewClientWithHeartbeat("ws://localhost:8765", nil)

	ctx := context.Background()

	// Connect to daemon
	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer client.Close()

	// Start receiving messages (heartbeat events will be processed automatically)
	eventCh, err := client.ReceiveMessages(ctx)
	if err != nil {
		fmt.Printf("ReceiveMessages error: %v\n", err)
		return
	}

	// Bootstrap loop session (call before relying on ReceiveMessages for app events)
	loopID, err := soothe.BootstrapLoopSession(ctx, client, "", "/tmp/workspace", nil)
	if err != nil {
		fmt.Printf("Bootstrap error: %v\n", err)
		return
	}

	fmt.Printf("Loop ID: %s\n", loopID)

	// Check daemon health before sending a long-running query
	health := client.GetDaemonHealth()
	if health != nil {
		fmt.Printf("Daemon state: %s\n", health.State)
		fmt.Printf("Daemon alive: %v\n", health.IsAlive)
		fmt.Printf("Last heartbeat: %v\n", health.LastHeartbeat)
	}

	// Send input that might take a long time to process
	if err := client.SendInput(ctx, "Analyze this large codebase", soothe.WithLoopID(loopID)); err != nil {
		fmt.Printf("Send error: %v\n", err)
		return
	}

	// Process events while monitoring daemon health
	// Heartbeats will automatically update the tracker during long operations
	tracker := client.GetHeartbeatTracker()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-eventCh:
			if msg == nil {
				return
			}

			// Check daemon health periodically
			if tracker != nil {
				fmt.Printf("Daemon processing: %v\n", tracker.IsProcessing())
			}

			// Process event
			// ... handle event based on type ...
		}
	}
}

// Example_waitForDaemonAlive demonstrates waiting for daemon to be alive before proceeding.
func Example_waitForDaemonAlive() {
	client := soothe.NewClientWithHeartbeat("ws://localhost:8765", nil)

	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer client.Close()

	eventCh, err := client.ReceiveMessages(ctx)
	if err != nil {
		fmt.Printf("ReceiveMessages error: %v\n", err)
		return
	}

	// Wait for daemon to be alive before proceeding
	tracker := client.GetHeartbeatTracker()
	if tracker != nil {
		// Wait up to 30 seconds for daemon to be alive
		if err := tracker.WaitForAlive(30 * time.Second); err != nil {
			fmt.Printf("Daemon not alive: %v\n", err)
			return
		}
		fmt.Println("Daemon is alive!")
	}

	// Bootstrap and proceed with normal operations
	loopID, err := soothe.BootstrapLoopSession(ctx, client, "", "/tmp/workspace", nil)
	if err != nil {
		fmt.Printf("Bootstrap error: %v\n", err)
		return
	}
	_ = eventCh // background reader for heartbeat traffic

	fmt.Printf("Loop ID: %s\n", loopID)
}

// Example_customHeartbeatThreshold demonstrates using a custom alive threshold.
func Example_customHeartbeatThreshold() {
	// Create client with custom heartbeat threshold (25 seconds instead of default 15)
	client := soothe.NewClient("ws://localhost:8765", nil)
	client.EnableHeartbeatTrackingWithThreshold(25 * time.Second)

	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer client.Close()

	// ... rest of client usage ...

	// Check if daemon is alive with custom threshold
	if client.IsDaemonAlive() {
		fmt.Println("Daemon is alive (within 25 second threshold)")
	}
}

// Example_heartbeatStateMonitoring demonstrates monitoring daemon state changes.
func Example_heartbeatStateMonitoring() {
	client := soothe.NewClientWithHeartbeat("ws://localhost:8765", nil)

	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer client.Close()

	eventCh, err := client.ReceiveMessages(ctx)
	if err != nil {
		fmt.Printf("ReceiveMessages error: %v\n", err)
		return
	}

	tracker := client.GetHeartbeatTracker()

	// Monitor daemon state changes
	lastState := ""
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-eventCh:
			if msg == nil {
				return
			}

			// Check for state changes
			if tracker != nil {
				currentState := tracker.GetState()
				if currentState != lastState {
					fmt.Printf("Daemon state changed: %s -> %s\n", lastState, currentState)
					lastState = currentState

					// React to state changes
					if currentState == "idle" {
						fmt.Println("Daemon is now idle, ready for new queries")
					} else if currentState == "running" {
						fmt.Printf("Daemon is processing loop: %s\n", tracker.GetLoopID())
					}
				}
			}

			// Process event
			// ... handle event ...
		}
	}
}