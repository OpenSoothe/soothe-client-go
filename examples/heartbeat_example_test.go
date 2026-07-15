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
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClientWithHeartbeat(md.URL, nil)

	ctx := context.Background()

	// Connect to daemon
	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	// Start receiving messages (heartbeat events will be processed automatically)
	_, err := client.ReceiveMessages(ctx)
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
	fmt.Printf("Daemon alive: %v\n", client.IsDaemonAlive())

	// Send input that might take a long time to process
	if err := client.SendInput(ctx, "Analyze this large codebase", soothe.WithLoopID(loopID)); err != nil {
		fmt.Printf("Send error: %v\n", err)
		return
	}
	fmt.Println("Input sent with heartbeat tracking")
	// Output:
	// Loop ID: loop-1
	// Daemon alive: true
	// Input sent with heartbeat tracking
}

// Example_waitForDaemonAlive demonstrates waiting for daemon to be alive before proceeding.
func Example_waitForDaemonAlive() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClientWithHeartbeat(md.URL, nil)

	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	eventCh, err := client.ReceiveMessages(ctx)
	if err != nil {
		fmt.Printf("ReceiveMessages error: %v\n", err)
		return
	}

	// Wait for daemon to be alive before proceeding
	tracker := client.GetHeartbeatTracker()
	if tracker != nil {
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
	// Output:
	// Daemon is alive!
	// Loop ID: loop-1
}

// Example_customHeartbeatThreshold demonstrates using a custom alive threshold.
func Example_customHeartbeatThreshold() {
	md := NewMockDaemon(nil)
	defer md.Close()

	// Create client with custom heartbeat threshold (25 seconds instead of default 15)
	client := soothe.NewClient(md.URL, nil)
	client.EnableHeartbeatTrackingWithThreshold(25 * time.Second)

	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	// ... rest of client usage ...

	// Check if daemon is alive with custom threshold
	if client.IsDaemonAlive() {
		fmt.Println("Daemon is alive (within 25 second threshold)")
	}
	// Output:
	// Daemon is alive (within 25 second threshold)
}

// Example_heartbeatStateMonitoring demonstrates monitoring daemon state changes.
func Example_heartbeatStateMonitoring() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClientWithHeartbeat(md.URL, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	eventCh, err := client.ReceiveMessages(ctx)
	if err != nil {
		fmt.Printf("ReceiveMessages error: %v\n", err)
		return
	}

	tracker := client.GetHeartbeatTracker()
	if tracker == nil {
		fmt.Println("No heartbeat tracker")
		return
	}

	// Monitor daemon state changes until context expires.
	// In a real deployment, the daemon sends periodic status frames with state
	// changes (idle -> running -> idle). The mock daemon doesn't send unsolicited
	// status frames, so no state transitions are observed here.
	lastState := tracker.GetState()
	fmt.Printf("Initial state: %q\n", lastState)
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case msg := <-eventCh:
			if msg == nil {
				break loop
			}
			currentState := tracker.GetState()
			if currentState != lastState {
				fmt.Printf("Daemon state: %s\n", currentState)
				lastState = currentState
			}
		}
	}
	fmt.Println("Monitoring complete")
	// Output:
	// Initial state: ""
	// Monitoring complete
}
