package soothe

import (
	"context"
	"testing"
	"time"
)

// integrationTestConfig returns config suitable for short integration runs.
func integrationTestConfig() *Config {
	c := DefaultConfig()
	c.DaemonReadyTimeout = 30 * time.Second
	c.LoopStatusTimeout = 60 * time.Second
	c.SubscriptionTimeout = 30 * time.Second
	return c
}

// skipIfShort skips integration tests when -short flag is set.
func skipIfShort(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration test requires running Soothe daemon")
	}
}

// skipIfNoDaemon skips integration tests if daemon is not running.
func skipIfNoDaemon(t *testing.T) {
	skipIfShort(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client := NewClient("ws://localhost:8765", DefaultConfig())
	if err := client.Connect(ctx); err != nil {
		t.Skip("Soothe daemon not running at ws://localhost:8765")
	}
	client.Close()
}

// ---------------------------------------------------------------------------
// Basic connection tests
// ---------------------------------------------------------------------------

func TestIntegration_ConnectAndClose(t *testing.T) {
	skipIfShort(t)
	cfg := integrationTestConfig()
	client := NewClient(cfg.DaemonURL, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect to daemon at %s: %v", cfg.DaemonURL, err)
	}
	defer client.Close()

	if !client.IsConnected() {
		t.Error("Client should be connected after Connect()")
	}
	t.Log("Successfully connected to Soothe daemon")
}

func TestIntegration_DaemonReady(t *testing.T) {
	skipIfShort(t)
	cfg := integrationTestConfig()
	client := NewClient(cfg.DaemonURL, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	// Connect() performs the protocol-1 handshake; WaitForDaemonReady returns
	// immediately once the handshake reports readiness_state "ready".
	ev, err := client.WaitForDaemonReady(cfg.DaemonReadyTimeout)
	if err != nil {
		t.Fatalf("WaitForDaemonReady: %v", err)
	}
	t.Logf("Daemon is ready: %v", ev)
}

func TestIntegration_NewLoopCreation(t *testing.T) {
	skipIfShort(t)
	cfg := integrationTestConfig()
	client := NewClient(cfg.DaemonURL, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	wsDir := t.TempDir()
	loopID, err := BootstrapLoopSession(ctx, client, "", wsDir, cfg)
	if err != nil {
		t.Fatalf("BootstrapLoopSession: %v", err)
	}
	t.Logf("Created new loop: %s", loopID)

	_, err = client.ReceiveMessages(ctx)
	if err != nil {
		t.Fatalf("ReceiveMessages: %v", err)
	}
}

func TestIntegration_InputMessage(t *testing.T) {
	skipIfShort(t)
	cfg := integrationTestConfig()
	client := NewClient(cfg.DaemonURL, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	wsDir := t.TempDir()
	loopID, err := BootstrapLoopSession(ctx, client, "", wsDir, cfg)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	t.Logf("Loop ID: %s", loopID)

	eventCh, err := client.ReceiveMessages(ctx)
	if err != nil {
		t.Fatalf("ReceiveMessages: %v", err)
	}

	if err := client.SendInput(ctx, "Hello, this is a test message from Go client", WithLoopID(loopID)); err != nil {
		t.Fatalf("SendInput: %v", err)
	}
	t.Log("Sent input message")

	// Read some events
	eventCount := 0
	eventTimeout := time.After(10 * time.Second)
	for {
		select {
		case <-eventTimeout:
			t.Logf("Received %d events", eventCount)
			return
		case msg := <-eventCh:
			if msg == nil {
				continue
			}
			eventCount++
			switch m := msg.(type) {
			case EventMessage:
				t.Logf("Event #%d: mode=%s event_type=%s", eventCount, m.Mode, m.EventType())
			case ErrorResponse:
				t.Logf("Error: code=%s, message=%s", m.Code, m.Message)
			default:
				t.Logf("Event #%d: type=%T", eventCount, msg)
			}
		}
	}
}

func TestIntegration_DaemonStatus(t *testing.T) {
	skipIfShort(t)
	cfg := integrationTestConfig()
	client := NewClient(cfg.DaemonURL, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	// Connect() completes the protocol-1 handshake; the daemon is ready.
	if _, err := client.WaitForDaemonReady(cfg.DaemonReadyTimeout); err != nil {
		t.Fatalf("WaitForDaemonReady: %v", err)
	}

	resp, err := client.RequestResponse(ctx, map[string]interface{}{
		"type": "daemon_status",
	}, "daemon_status_response", 5*time.Second)
	if err != nil {
		t.Fatalf("RequestResponse daemon_status: %v", err)
	}
	t.Logf("Daemon status: running=%v, port_live=%v, active_loops=%v",
		resp["running"], resp["port_live"], resp["active_loops"])
}

func TestIntegration_SkillsList(t *testing.T) {
	skipIfShort(t)
	cfg := integrationTestConfig()
	client := NewClient(cfg.DaemonURL, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	// Connect() completes the protocol-1 handshake; the daemon is ready.
	if _, err := client.WaitForDaemonReady(cfg.DaemonReadyTimeout); err != nil {
		t.Fatalf("WaitForDaemonReady: %v", err)
	}

	resp, err := client.ListSkills(ctx, 15*time.Second)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	t.Logf("Skills response: %v", resp)
}

func TestIntegration_ModelsList(t *testing.T) {
	skipIfShort(t)
	cfg := integrationTestConfig()
	client := NewClient(cfg.DaemonURL, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	// Connect() completes the protocol-1 handshake; the daemon is ready.
	if _, err := client.WaitForDaemonReady(cfg.DaemonReadyTimeout); err != nil {
		t.Fatalf("WaitForDaemonReady: %v", err)
	}

	resp, err := client.ListModels(ctx, 15*time.Second)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	t.Logf("Models response: %v", resp)
}

func TestIntegration_IsDaemonLive(t *testing.T) {
	skipIfShort(t)
	cfg := integrationTestConfig()

	if !IsDaemonLive(cfg.DaemonURL, 10*time.Second) {
		t.Fatal("Expected daemon to be live")
	}
	t.Log("Daemon is live")
}

func TestIntegration_ConnectionRecovery(t *testing.T) {
	skipIfShort(t)
	cfg := integrationTestConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client1 := NewClient(cfg.DaemonURL, cfg)
	if err := client1.Connect(ctx); err != nil {
		t.Fatalf("Initial connection failed: %v", err)
	}
	t.Log("Initial connection established")
	client1.Close()
	if client1.IsConnected() {
		t.Error("Client should not be connected after Close()")
	}

	client2 := NewClient(cfg.DaemonURL, cfg)
	if err := client2.Connect(ctx); err != nil {
		t.Fatalf("Second client connection failed: %v", err)
	}
	if !client2.IsConnected() {
		t.Error("Second client should be connected")
	}
	t.Log("Successfully connected with new client after previous close")
	client2.Close()
}

func TestIntegration_ConfigGet(t *testing.T) {
	skipIfShort(t)
	cfg := integrationTestConfig()
	client := NewClient(cfg.DaemonURL, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	// Connect() completes the protocol-1 handshake; the daemon is ready.
	if _, err := client.WaitForDaemonReady(cfg.DaemonReadyTimeout); err != nil {
		t.Fatalf("WaitForDaemonReady: %v", err)
	}

	config, err := FetchConfigSection(ctx, client, "providers", 5*time.Second)
	if err != nil {
		t.Fatalf("FetchConfigSection: %v", err)
	}
	t.Logf("Config section: %v", config)
}

func TestIntegration_FullConversation(t *testing.T) {
	skipIfShort(t)
	cfg := integrationTestConfig()
	client := NewClient(cfg.DaemonURL, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	wsDir := t.TempDir()
	loopID, err := BootstrapLoopSession(ctx, client, "", wsDir, cfg)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	t.Logf("Loop ID: %s", loopID)

	eventCh, err := client.ReceiveMessages(ctx)
	if err != nil {
		t.Fatalf("ReceiveMessages: %v", err)
	}

	if err := client.SendInput(ctx, "List all files in the current directory", WithLoopID(loopID)); err != nil {
		t.Fatalf("Input: %v", err)
	}

	// Stream events for 20 seconds
	eventTypes := make(map[string]int)
	streamTimeout := time.After(20 * time.Second)

	for {
		select {
		case <-streamTimeout:
			t.Logf("Event streaming completed")
			for eventType, count := range eventTypes {
				t.Logf("  %s: %d", eventType, count)
			}
			return
		case msg := <-eventCh:
			if msg == nil {
				continue
			}
			switch m := msg.(type) {
			case EventMessage:
				eventType := m.EventType()
				if eventType == "" {
					eventType = "event"
				}
				eventTypes[eventType]++
			default:
				eventTypes["other"]++
			}
		}
	}
}

func TestIntegration_SendDetach(t *testing.T) {
	skipIfShort(t)
	cfg := integrationTestConfig()
	client := NewClient(cfg.DaemonURL, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	// Connect() completes the protocol-1 handshake; the daemon is ready.
	if _, err := client.WaitForDaemonReady(cfg.DaemonReadyTimeout); err != nil {
		t.Fatalf("WaitForDaemonReady: %v", err)
	}

	if err := client.SendDetach(ctx); err != nil {
		t.Fatalf("SendDetach: %v", err)
	}
	t.Log("Sent detach message")
}

func TestIntegration_CheckDaemonStatus(t *testing.T) {
	skipIfShort(t)
	cfg := integrationTestConfig()
	client := NewClient(cfg.DaemonURL, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	// Connect() completes the protocol-1 handshake; the daemon is ready.
	if _, err := client.WaitForDaemonReady(cfg.DaemonReadyTimeout); err != nil {
		t.Fatalf("WaitForDaemonReady: %v", err)
	}

	resp, err := CheckDaemonStatus(ctx, client, 5*time.Second)
	if err != nil {
		t.Fatalf("CheckDaemonStatus: %v", err)
	}
	t.Logf("Daemon status: %v", resp)
}

func TestIntegration_FetchSkillsCatalog(t *testing.T) {
	skipIfShort(t)
	cfg := integrationTestConfig()
	client := NewClient(cfg.DaemonURL, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	// Connect() completes the protocol-1 handshake; the daemon is ready.
	if _, err := client.WaitForDaemonReady(cfg.DaemonReadyTimeout); err != nil {
		t.Fatalf("WaitForDaemonReady: %v", err)
	}

	skills, err := FetchSkillsCatalog(ctx, client, 15*time.Second)
	if err != nil {
		t.Fatalf("FetchSkillsCatalog: %v", err)
	}
	t.Logf("Skills catalog: %d skills", len(skills))
	for _, s := range skills {
		t.Logf("  - %v", s["name"])
	}
}

// ---------------------------------------------------------------------------
// IntentHint integration tests
// ---------------------------------------------------------------------------

func TestIntegration_InputWithIntentHintQuiz(t *testing.T) {
	skipIfShort(t)
	cfg := integrationTestConfig()
	client := NewClient(cfg.DaemonURL, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	wsDir := t.TempDir()
	loopID, err := BootstrapLoopSession(ctx, client, "", wsDir, cfg)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	t.Logf("Loop ID: %s", loopID)

	eventCh, err := client.ReceiveMessages(ctx)
	if err != nil {
		t.Fatalf("ReceiveMessages: %v", err)
	}

	// Send input with intent_hint=quiz to use think model for factual response
	if err := client.SendInput(ctx, "What is the capital of France?", WithLoopID(loopID), WithIntentHint("quiz")); err != nil {
		t.Fatalf("SendInput with intent_hint=quiz: %v", err)
	}
	t.Log("Sent input message with intent_hint=quiz")

	// Read events until we get an assistant response or timeout
	eventCount := 0
	eventTimeout := time.After(10 * time.Second)
	for {
		select {
		case <-eventTimeout:
			t.Logf("Received %d events", eventCount)
			return
		case msg := <-eventCh:
			if msg == nil {
				continue
			}
			eventCount++
			switch m := msg.(type) {
			case EventMessage:
				t.Logf("Event #%d: mode=%s event_type=%s", eventCount, m.Mode, m.EventType())
			case ErrorResponse:
				t.Logf("Error: code=%s, message=%s", m.Code, m.Message)
			default:
				t.Logf("Event #%d: type=%T", eventCount, msg)
			}
		}
	}
}
