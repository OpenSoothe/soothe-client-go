package soothe_test

import (
	"context"
	"fmt"
	"time"

	soothe "github.com/mirasoth/soothe-client-go"
)

// Example_basicConnection demonstrates the minimal connect → bootstrap →
// send → close lifecycle that most client programs follow.
func Example_basicConnection() {
	// Use DefaultConfig for sensible timeouts and retry defaults.
	cfg := soothe.DefaultConfig()
	cfg.DaemonReadyTimeout = 30 * time.Second

	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, cfg)
	ctx := context.Background()

	// Connect performs the protocol-1 handshake (connection_init / connection_ack).
	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer client.Close()

	// Check readiness after the handshake.
	fmt.Printf("Handshake complete: %v\n", client.IsHandshakeComplete())
	fmt.Printf("Readiness state: %s\n", client.ReadinessState())

	// Bootstrap a fresh loop session (loop_new + loop_subscribe).
	loopID, err := soothe.BootstrapLoopSession(ctx, client, "", "/tmp/workspace", nil)
	if err != nil {
		fmt.Printf("Bootstrap error: %v\n", err)
		return
	}
	fmt.Printf("Loop ID: %s\n", loopID)

	// Send a fire-and-forget input notification.
	if err := client.SendInput(ctx, "Hello, Soothe!", soothe.WithLoopID(loopID)); err != nil {
		fmt.Printf("Send error: %v\n", err)
	}
	// Output:
	// Handshake complete: true
	// Readiness state: ready
	// Loop ID: loop-1
}

// Example_connectWithRetries handles cold-start races where the daemon may
// not be ready yet. ConnectWithRetries wraps Connect with bounded retry logic.
func Example_connectWithRetries() {
	md := NewMockDaemon(nil)
	defer md.Close()

	cfg := soothe.DefaultConfig()
	client := soothe.NewClient(md.URL, cfg)
	ctx := context.Background()

	// Retry up to 10 times with 500ms delay (handles daemon cold start).
	if err := soothe.ConnectWithRetries(ctx, client, 10, 500*time.Millisecond); err != nil {
		fmt.Printf("Connect failed: %v\n", err)
		return
	}
	defer client.Close()

	fmt.Printf("Connected: %v\n", client.IsConnected())
	// Output:
	// Connected: true
}

// Example_loadConfigFromEnv shows configuration from environment variables:
// SOOTHE_DAEMON_URL, SOOTHE_VERBOSITY, SOOTHE_MAX_RETRIES, etc.
func Example_loadConfigFromEnv() {
	cfg := soothe.LoadConfigFromEnv()
	fmt.Printf("Daemon URL: %s\n", cfg.DaemonURL)
	fmt.Printf("Verbosity: %s\n", cfg.VerbosityLevel)
	fmt.Printf("Max retries: %d\n", cfg.MaxRetries)
	// Output:
	// Daemon URL: ws://localhost:8765
	// Verbosity: normal
	// Max retries: 5
}

// Example_disconnectMonitoring watches the Disconnected channel for mid-session
// connection drops and distinguishes clean vs unclean loss.
func Example_disconnectMonitoring() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer client.Close()

	// Start the event reader so the read loop can detect drops.
	_, err := client.ReceiveMessages(ctx)
	if err != nil {
		fmt.Printf("ReceiveMessages error: %v\n", err)
		return
	}

	// Simulate a clean disconnect by closing the mock daemon.
	md.Close()

	// Disconnected() fires exactly once on drop.
	select {
	case cause := <-client.Disconnected():
		fmt.Printf("Connection dropped: %s\n", cause.String())
	case <-ctx.Done():
		fmt.Printf("Context cancelled before disconnect\n")
	}
	// Output:
	// Connection dropped: unclean
}

// Example_customConfig shows a Config with custom timeouts and reconnect
// backoff parameters (RFC-450 §8.3 dead-connection detection).
func Example_customConfig() {
	cfg := &soothe.Config{
		DaemonURL:             "ws://localhost:8765",
		VerbosityLevel:        "debug",
		MaxRetries:            10,
		ReconnectDelay:        1 * time.Second,
		HeartbeatInterval:     15 * time.Second,
		DaemonReadyTimeout:    30 * time.Second,
		LoopStatusTimeout:     90 * time.Second,
		SubscriptionTimeout:   15 * time.Second,
		ReconnectMaxAttempts:  20,
		ReconnectInitialDelay: 500 * time.Millisecond,
		ReconnectMaxDelay:     15 * time.Second,
		ReattachProbeTimeout:  10 * time.Second,
	}

	client := soothe.NewClient(cfg.DaemonURL, cfg)
	fmt.Printf("Client created for %s\n", cfg.DaemonURL)
	_ = client
	// Output:
	// Client created for ws://localhost:8765
}
