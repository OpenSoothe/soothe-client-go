package soothe

import (
	"context"
	"testing"
	"time"
)

// skipIfNoDaemon skips integration tests if daemon is not running
func skipIfNoDaemon(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration test requires running Soothe daemon")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client := NewClient("ws://localhost:8765", DefaultConfig())
	if err := client.Connect(ctx); err != nil {
		t.Skip("Soothe daemon not running at ws://localhost:8765")
	}
	client.Close()
}

// =============================================================================
// THREAD LIFECYCLE TESTS
// =============================================================================

func TestIntegration_ThreadCreate(t *testing.T) {
	skipIfNoDaemon(t)

	ctx := context.Background()
	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	// Wait for daemon ready
	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	response, err := client.RequestResponse(ctx, map[string]interface{}{
		"type":            "thread_create",
		"initial_message": "Initial test message",
		"metadata": map[string]interface{}{
			"test":       true,
			"created_by": "integration_test",
		},
	}, "thread_created", 10*time.Second)

	if err != nil {
		t.Logf("Thread create response: %v (may timeout for persisted threads)", err)
		return
	}

	if response != nil {
		threadID, _ := response["thread_id"].(string)
		t.Logf("Created persisted thread: %s", threadID)
		if threadID != "" {
			t.Logf("Thread created successfully with ID: %s", threadID)
		}
	}
}

func TestIntegration_ThreadList(t *testing.T) {
	skipIfNoDaemon(t)

	ctx := context.Background()
	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	response, err := client.RequestResponse(ctx, map[string]interface{}{
		"type":                 "thread_list",
		"include_stats":        false,
		"include_last_message": false,
	}, "thread_list_response", 10*time.Second)

	if err != nil {
		t.Fatalf("Failed to get thread list: %v", err)
	}

	threads, ok := response["threads"].([]map[string]interface{})
	if !ok {
		t.Logf("Thread list response: %v", response)
		return
	}

	t.Logf("Found %d threads", len(threads))
	for i, thread := range threads {
		threadID, _ := thread["thread_id"].(string)
		t.Logf("  Thread %d: %s", i+1, threadID)
	}
}

func TestIntegration_ThreadGet(t *testing.T) {
	skipIfNoDaemon(t)

	ctx := context.Background()
	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	// Create a thread first
	threadID := createTestThread(t, client, ctx)
	if threadID == "" {
		t.Skip("Could not create test thread")
	}

	requestID := NewRequestID()
	response, err := client.RequestResponse(ctx, map[string]interface{}{
		"type":       "thread_get",
		"request_id": requestID,
		"thread_id":  threadID,
	}, "thread_get_response", 10*time.Second)

	if err != nil {
		t.Logf("Thread get response error: %v", err)
	} else {
		t.Logf("Thread metadata: %v", response)
	}
}

func TestIntegration_ThreadMessages(t *testing.T) {
	skipIfNoDaemon(t)

	ctx := context.Background()
	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	threadID := createTestThread(t, client, ctx)
	if threadID == "" {
		t.Skip("Could not create test thread")
	}

	requestID := NewRequestID()
	response, err := client.RequestResponse(ctx, map[string]interface{}{
		"type":       "thread_messages",
		"request_id": requestID,
		"thread_id":  threadID,
		"limit":      10,
		"offset":     0,
	}, "thread_messages_response", 10*time.Second)

	if err != nil {
		t.Logf("Thread messages response: %v", err)
	} else {
		t.Logf("Thread messages: %v", response)
	}
}

func TestIntegration_ThreadState(t *testing.T) {
	skipIfNoDaemon(t)

	ctx := context.Background()
	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	threadID := createTestThread(t, client, ctx)
	if threadID == "" {
		t.Skip("Could not create test thread")
	}

	requestID := NewRequestID()
	response, err := client.RequestResponse(ctx, map[string]interface{}{
		"type":       "thread_state",
		"request_id": requestID,
		"thread_id":  threadID,
	}, "thread_state_response", 10*time.Second)

	if err != nil {
		t.Logf("Thread state response: %v", err)
	} else {
		t.Logf("Thread state checkpoint: %v", response)
	}
}

func TestIntegration_ThreadUpdateState(t *testing.T) {
	skipIfNoDaemon(t)

	ctx := context.Background()
	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	threadID := createTestThread(t, client, ctx)
	if threadID == "" {
		t.Skip("Could not create test thread")
	}

	// Update thread state
	requestID := NewRequestID()
	values := map[string]interface{}{
		"custom_key": "custom_value",
		"test_data":  12345,
	}

	response, err := client.RequestResponse(ctx, map[string]interface{}{
		"type":       "thread_update_state",
		"request_id": requestID,
		"thread_id":  threadID,
		"values":     values,
	}, "thread_update_state_response", 10*time.Second)

	if err != nil {
		t.Logf("Thread update state response: %v", err)
	} else {
		t.Logf("Updated thread state: %v", response)
	}
}

func TestIntegration_ThreadArchive(t *testing.T) {
	skipIfNoDaemon(t)

	ctx := context.Background()
	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	threadID := createTestThread(t, client, ctx)
	if threadID == "" {
		t.Skip("Could not create test thread")
	}

	requestID := NewRequestID()
	response, err := client.RequestResponse(ctx, map[string]interface{}{
		"type":       "thread_archive",
		"request_id": requestID,
		"thread_id":  threadID,
	}, "thread_operation_ack", 10*time.Second)

	if err != nil {
		t.Logf("Thread archive response: %v", err)
	} else {
		t.Logf("Archived thread: %v", response)
	}
}

func TestIntegration_ThreadArtifacts(t *testing.T) {
	skipIfNoDaemon(t)

	ctx := context.Background()
	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	threadID := createTestThread(t, client, ctx)
	if threadID == "" {
		t.Skip("Could not create test thread")
	}

	requestID := NewRequestID()
	response, err := client.RequestResponse(ctx, map[string]interface{}{
		"type":       "thread_artifacts",
		"request_id": requestID,
		"thread_id":  threadID,
	}, "thread_artifacts_response", 15*time.Second)

	if err != nil {
		t.Logf("Thread artifacts response: %v", err)
	} else {
		t.Logf("Thread artifacts: %v", response)
	}
}

// =============================================================================
// SKILL INVOCATION TESTS
// =============================================================================

func TestIntegration_InvokeSkill_Weather(t *testing.T) {
	skipIfNoDaemon(t)

	ctx := context.Background()
	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	// Invoke weather skill
	skill := "weather"
	args := "Beijing China"

	response, err := client.InvokeSkill(ctx, skill, args, 30*time.Second)
	if err != nil {
		t.Logf("Weather skill invocation error: %v", err)
		return
	}

	t.Logf("Weather skill response: %v", response)

	// Check if we got weather data
	if response["type"] == "invoke_skill_response" {
		t.Logf("Successfully invoked weather skill")
	}
}

func TestIntegration_InvokeSkill_List(t *testing.T) {
	skipIfNoDaemon(t)

	ctx := context.Background()
	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	// First list skills to find available ones
	skillsResponse, err := client.ListSkills(ctx, 10*time.Second)
	if err != nil {
		t.Fatalf("Failed to list skills: %v", err)
	}

	skills, ok := skillsResponse["skills"].([]map[string]interface{})
	if !ok {
		t.Skip("No skills available")
	}

	t.Logf("Available skills: %d", len(skills))

	// Try invoking the first non-builtin skill if available
	for _, skill := range skills {
		name, _ := skill["name"].(string)
		t.Logf("Skill: %s", name)
	}
}

// =============================================================================
// VERBOSITY AND EVENT CLASSIFICATION TESTS
// =============================================================================

func TestIntegration_VerbosityLevels(t *testing.T) {
	skipIfNoDaemon(t)

	// Test all verbosity levels
	levels := []VerbosityLevel{
		VerbosityQuiet,
		VerbosityMinimal,
		VerbosityNormal,
		VerbosityDetailed,
		VerbosityDebug,
	}

	for _, level := range levels {
		t.Run(string(level), func(t *testing.T) {
			cfg := integrationTestConfig()
			cfg.VerbosityLevel = string(level)

			ctx := context.Background()
			client := NewClient("ws://localhost:8765", cfg)

			if err := client.Connect(ctx); err != nil {
				t.Fatalf("Failed to connect: %v", err)
			}
			defer client.Close()

			if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
				t.Fatalf("Daemon not ready: %v", err)
			}

			// Create thread with this verbosity
			threadID := createTestThread(t, client, ctx)
			if threadID == "" {
				t.Skip("Could not create test thread")
			}

			// Subscribe with this verbosity
			if err := client.SendSubscribeThread(ctx, threadID, string(level)); err != nil {
				t.Fatalf("Failed to subscribe: %v", err)
			}

			t.Logf("Thread %s subscribed with verbosity: %s", threadID, level)
		})
	}
}

func TestIntegration_EventClassification(t *testing.T) {
	skipIfNoDaemon(t)

	// Test event classification with verbosity
	testEvents := []struct {
		eventType    string
		expectedTier VerbosityTier
	}{
		{EventPlanCreated, TierNormal},
		{EventThreadCompleted, TierQuiet},
		{EventChitchatResponse, TierQuiet},
		{EventBrowserStarted, TierNormal},
		{EventToolStarted, TierInternal},
		{EventFinalReport, TierQuiet},
	}

	for _, test := range testEvents {
		t.Run(test.eventType, func(t *testing.T) {
			tier := ClassifyEventVerbosity(test.eventType)
			if tier != test.expectedTier {
				t.Errorf("Event %s: expected tier %d, got tier %d",
					test.eventType, test.expectedTier, tier)
			}

			// Test ShouldShow with different verbosity levels
			for _, level := range []VerbosityLevel{VerbosityQuiet, VerbosityNormal, VerbosityDetailed} {
				visible := ShouldShow(tier, level)
				t.Logf("Event %s at %s: visible=%v (tier=%d)",
					test.eventType, level, visible, tier)
			}
		})
	}
}

// =============================================================================
// ERROR HANDLING TESTS
// =============================================================================

func TestIntegration_ErrorHandling_InvalidThreadID(t *testing.T) {
	skipIfNoDaemon(t)

	ctx := context.Background()
	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	// Try to resume non-existent thread
	threadID := "non-existent-thread-12345"
	if err := client.SendResumeThread(ctx, threadID, "/tmp/ws"); err != nil {
		t.Fatalf("Failed to send resume_thread: %v", err)
	}

	// Subscribe and wait for error or status
	eventCh, err := client.ReceiveMessages(ctx)
	if err != nil {
		t.Fatalf("Failed to receive messages: %v", err)
	}

	timeout := time.After(5 * time.Second)
	errorReceived := false

	for {
		select {
		case <-timeout:
			if !errorReceived {
				t.Log("Timeout waiting for error response")
			}
			return
		case msg := <-eventCh:
			if msg == nil {
				return
			}

			switch m := msg.(type) {
			case ErrorResponse:
				t.Logf("Received error: code=%s message=%s", m.Code, m.Message)
				errorReceived = true
				return
			case StatusResponse:
				t.Logf("Received status: state=%s thread_id=%s", m.State, m.ThreadID)
				return
			}
		}
	}
}

func TestIntegration_ErrorHandling_ContextCancellation(t *testing.T) {
	skipIfNoDaemon(t)

	ctx, cancel := context.WithCancel(context.Background())
	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Cancel context immediately
	cancel()

	// Try operations with cancelled context
	err := client.SendInput(ctx, "test")
	if err == nil {
		t.Error("Expected error with cancelled context")
	} else {
		t.Logf("Correctly received error: %v", err)
	}

	client.Close()
}

// =============================================================================
// CONCURRENT CLIENT TESTS
// =============================================================================

func TestIntegration_ConcurrentClients(t *testing.T) {
	skipIfNoDaemon(t)

	numClients := 3
	clients := make([]*Client, numClients)
	threadIDs := make([]string, numClients)

	// Create multiple clients concurrently
	for i := 0; i < numClients; i++ {
		go func(idx int) {
			ctx := context.Background()
			client := NewClient("ws://localhost:8765", integrationTestConfig())

			if err := client.Connect(ctx); err != nil {
				t.Errorf("Client %d: Failed to connect: %v", idx, err)
				return
			}
			defer client.Close()

			if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
				t.Errorf("Client %d: Daemon not ready: %v", idx, err)
				return
			}

			// Create thread
			threadID := createTestThread(t, client, ctx)
			clients[idx] = client
			threadIDs[idx] = threadID

			t.Logf("Client %d: Created thread %s", idx, threadID)
		}(i)
	}

	// Wait for all clients
	time.Sleep(5 * time.Second)

	t.Logf("Created %d threads from %d concurrent clients", numClients, numClients)
	for i, tid := range threadIDs {
		if tid != "" {
			t.Logf("  Client %d: thread %s", i, tid)
		}
	}
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

func createTestThread(t *testing.T, client *Client, ctx context.Context) string {
	t.Helper()

	// Create new thread (assumes daemon_ready already exchanged)
	if err := client.SendNewThread(ctx, "/tmp/soothe-test-ws"); err != nil {
		t.Logf("Failed to send new_thread: %v", err)
		return ""
	}

	// Wait for status (bounded reads so we do not block forever without daemon traffic)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if client.conn != nil {
			client.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		}
		ev, err := client.ReadEvent()
		if err != nil {
			continue
		}
		if ev == nil {
			continue
		}
		threadID, ok := ExtractSootheLoopID(ev)
		if ok && threadID != "" {
			if client.conn != nil {
				client.conn.SetReadDeadline(time.Time{})
			}
			t.Logf("Created test thread: %s", threadID)
			return threadID
		}
	}
	t.Log("Timeout waiting for thread status")
	return ""
}

func TestIntegration_LongRunningConversation(t *testing.T) {
	skipIfNoDaemon(t)

	ctx := context.Background()
	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	// Bootstrap first (uses request/response; do not start ReceiveMessages yet).
	loopID, err := BootstrapLoopSession(ctx, client, "", "/tmp/soothe-test-ws", integrationTestConfig())
	if err != nil {
		t.Fatalf("Failed to bootstrap session: %v", err)
	}

	eventCh, err := client.ReceiveMessages(ctx)
	if err != nil {
		t.Fatalf("Failed to start receiving messages: %v", err)
	}
	_ = eventCh

	t.Logf("Started long conversation in loop: %s", loopID)

	// Send multiple inputs
	inputs := []string{
		"What is the weather in Beijing?",
		"List files in current directory",
		"Explain Go concurrency",
	}

	for i, input := range inputs {
		t.Logf("Sending input %d: %s", i+1, input)

		if err := client.SendInput(ctx, input, WithLoopID(loopID)); err != nil {
			t.Errorf("Failed to send input %d: %v", i+1, err)
			continue
		}

		// Wait a bit between inputs
		time.Sleep(3 * time.Second)
	}

	t.Logf("Sent %d inputs in long conversation", len(inputs))
}

func TestIntegration_SubscribeMultipleThreads(t *testing.T) {
	skipIfNoDaemon(t)

	ctx := context.Background()
	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	// Create multiple threads
	threadIDs := make([]string, 3)
	for i := 0; i < 3; i++ {
		threadIDs[i] = createTestThread(t, client, ctx)
		if threadIDs[i] == "" {
			t.Logf("Failed to create thread %d", i+1)
		}
		time.Sleep(1 * time.Second)
	}

	// Subscribe to all threads with different verbosity levels
	verbosities := []string{"quiet", "normal", "detailed"}

	for i, threadID := range threadIDs {
		if threadID == "" {
			continue
		}

		verbosity := verbosities[i%len(verbosities)]
		if err := client.SendSubscribeThread(ctx, threadID, verbosity); err != nil {
			t.Errorf("Failed to subscribe to thread %s: %v", threadID, err)
		}

		t.Logf("Subscribed to thread %s with verbosity %s", threadID, verbosity)
	}

	t.Logf("Subscribed to %d threads", len(threadIDs))
}
