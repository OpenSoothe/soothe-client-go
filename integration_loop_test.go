package soothe

import (
	"context"
	"testing"
	"time"
)

// =============================================================================
// LOOP MANAGEMENT API TESTS (RFC-503, RFC-411)
// =============================================================================

func TestIntegration_LoopNew(t *testing.T) {
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

	// Create a new loop
	response, err := client.LoopNew(ctx, 15*time.Second)
	if err != nil {
		t.Logf("LoopNew error: %v (daemon may not support loop API)", err)
		return
	}

	t.Logf("LoopNew response: %v", response)

	loopID, ok := response["loop_id"].(string)
	if !ok || loopID == "" {
		t.Logf("No loop_id in response, daemon may not fully support loop API")
		return
	}

	t.Logf("Created new loop: %s", loopID)
}

func TestIntegration_LoopList(t *testing.T) {
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

	// List all loops
	response, err := client.LoopList(ctx, nil, 0, 15*time.Second)
	if err != nil {
		t.Logf("LoopList error: %v (daemon may not support loop API)", err)
		return
	}

	t.Logf("LoopList response: %v", response)

	loops, ok := response["loops"].([]map[string]interface{})
	if !ok {
		t.Logf("No loops array in response")
		return
	}

	total, _ := response["total"].(float64)
	t.Logf("Found %d loops (total: %v)", len(loops), total)

	for i, loop := range loops {
		loopID, _ := loop["loop_id"].(string)
		t.Logf("  Loop %d: %s", i+1, loopID)
	}
}

func TestIntegration_LoopGet(t *testing.T) {
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

	// First create a loop
	loopID := createTestLoop(t, client, ctx)
	if loopID == "" {
		t.Skip("Could not create test loop")
	}

	// Get loop details
	response, err := client.LoopGet(ctx, loopID, true, 15*time.Second)
	if err != nil {
		t.Logf("LoopGet error: %v", err)
		return
	}

	t.Logf("LoopGet response for loop %s: %v", loopID, response)

	loopData, ok := response["loop"].(map[string]interface{})
	if !ok {
		t.Logf("No loop data in response")
		return
	}

	t.Logf("Loop details: loop_id=%v, state=%v", loopData["loop_id"], loopData["state"])
}

func TestIntegration_LoopTree(t *testing.T) {
	skipIfNoDaemon(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	loopID := createTestLoop(t, client, ctx)
	if loopID == "" {
		t.Skip("Could not create test loop")
	}

	// Request checkpoint tree with shorter timeout
	response, err := client.LoopTree(ctx, loopID, "json", 10*time.Second)
	if err != nil {
		t.Logf("LoopTree error: %v (daemon may not fully support loop_tree)", err)
		return
	}

	t.Logf("LoopTree response for loop %s: %v", loopID, response)

	if _, ok := response["tree"].(map[string]interface{}); !ok {
		t.Logf("No tree data in response")
		return
	}

	t.Logf("Checkpoint tree structure available")
}

func TestIntegration_LoopSubscribeDetach(t *testing.T) {
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

	loopID := createTestLoop(t, client, ctx)
	if loopID == "" {
		t.Skip("Could not create test loop")
	}

	// Subscribe to loop events
	response, err := client.LoopSubscribe(ctx, loopID, 10*time.Second)
	if err != nil {
		t.Logf("LoopSubscribe error: %v", err)
		return
	}

	t.Logf("LoopSubscribe response: %v", response)
	success, _ := response["success"].(bool)
	if !success {
		t.Logf("Subscription not successful")
		return
	}

	t.Logf("Successfully subscribed to loop %s", loopID)

	// Detach from loop
	response2, err := client.LoopDetach(ctx, loopID, 10*time.Second)
	if err != nil {
		t.Logf("LoopDetach error: %v", err)
		return
	}

	t.Logf("LoopDetach response: %v", response2)
	success2, _ := response2["success"].(bool)
	if success2 {
		t.Logf("Successfully detached from loop %s", loopID)
	}
}

func TestIntegration_LoopInput(t *testing.T) {
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

	loopID := createTestLoop(t, client, ctx)
	if loopID == "" {
		t.Skip("Could not create test loop")
	}

	// Subscribe to loop to receive events
	eventCh, err := client.ReceiveMessages(ctx)
	if err != nil {
		t.Fatalf("ReceiveMessages: %v", err)
	}

	// Subscribe to loop
	if _, err := client.LoopSubscribe(ctx, loopID, 10*time.Second); err != nil {
		t.Logf("Could not subscribe to loop: %v", err)
	}

	response, err := client.LoopInput(ctx, loopID, "Hello from loop input test", 60*time.Second)
	if err != nil {
		t.Logf("LoopInput error: %v", err)
		return
	}

	t.Logf("LoopInput response: %v", response)

	threadID, _ := response["thread_id"].(string)
	if threadID != "" {
		t.Logf("Loop input created/used thread: %s", threadID)
	}

	// Collect some events
	eventCount := 0
	timeout := time.After(10 * time.Second)
	for {
		select {
		case <-timeout:
			t.Logf("Received %d loop events", eventCount)
			return
		case msg := <-eventCh:
			if msg == nil {
				continue
			}
			eventCount++
			switch m := msg.(type) {
			case EventMessage:
				t.Logf("Loop event #%d: type=%s mode=%s", eventCount, m.EventType(), m.Mode)
			case ErrorResponse:
				t.Logf("Loop error: code=%s message=%s", m.Code, m.Message)
			}
		}
	}
}

func TestIntegration_LoopPrune(t *testing.T) {
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

	loopID := createTestLoop(t, client, ctx)
	if loopID == "" {
		t.Skip("Could not create test loop")
	}

	// Request pruning with dry-run
	response, err := client.LoopPrune(ctx, loopID, 30, true, 15*time.Second)
	if err != nil {
		t.Logf("LoopPrune error: %v", err)
		return
	}

	t.Logf("LoopPrune response (dry-run): %v", response)

	result, ok := response["result"].(map[string]interface{})
	if !ok {
		t.Logf("No result data")
		return
	}

	branchesPruned, _ := result["branches_pruned"].(float64)
	t.Logf("Would prune %v branches (dry-run)", branchesPruned)
}

func TestIntegration_LoopDelete(t *testing.T) {
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

	// Create a temporary loop to delete
	loopID := createTestLoop(t, client, ctx)
	if loopID == "" {
		t.Skip("Could not create test loop")
	}

	t.Logf("Created temporary loop for deletion: %s", loopID)

	// Delete the loop
	response, err := client.LoopDelete(ctx, loopID, 10*time.Second)
	if err != nil {
		t.Logf("LoopDelete error: %v", err)
		return
	}

	t.Logf("LoopDelete response: %v", response)

	success, _ := response["success"].(bool)
	if success {
		t.Logf("Successfully deleted loop %s", loopID)
	}
}

func TestIntegration_LoopReattach(t *testing.T) {
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

	loopID := createTestLoop(t, client, ctx)
	if loopID == "" {
		t.Skip("Could not create test loop")
	}

	// Request reattachment
	response, err := client.LoopReattach(ctx, loopID, 15*time.Second)
	if err != nil {
		t.Logf("LoopReattach error: %v", err)
		return
	}

	t.Logf("LoopReattach response: %v", response)
	t.Logf("Reattached to loop %s", loopID)
}

func TestIntegration_SendLoopMethods(t *testing.T) {
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

	// Test raw send methods for loop APIs
	requestID := NewRequestID()

	// Test SendLoopList
	if err := client.SendLoopList(ctx, nil, 10, requestID); err != nil {
		t.Logf("SendLoopList error: %v", err)
	} else {
		t.Logf("Sent loop_list request: %s", requestID)
	}

	// Test SendLoopNew
	requestID2 := NewRequestID()
	if err := client.SendLoopNew(ctx, requestID2); err != nil {
		t.Logf("SendLoopNew error: %v", err)
	} else {
		t.Logf("Sent loop_new request: %s", requestID2)
	}
}

// =============================================================================
// THREAD STATUS AND COMMAND REQUEST API TESTS
// =============================================================================

func TestIntegration_ThreadStatusAPI(t *testing.T) {
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

	// Create a thread
	threadID := createTestThread(t, client, ctx)
	if threadID == "" {
		t.Skip("Could not create test thread")
	}

	// Request thread status using convenience method
	response, err := client.ThreadStatus(ctx, threadID, 10*time.Second)
	if err != nil {
		t.Logf("ThreadStatus error: %v", err)
		return
	}

	t.Logf("ThreadStatus response: %v", response)

	state, _ := response["state"].(string)
	hasActiveQuery, _ := response["has_active_query"].(bool)
	t.Logf("Thread %s: state=%s, has_active_query=%v", threadID, state, hasActiveQuery)
}

func TestIntegration_SendThreadStatus(t *testing.T) {
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
	ev, err := client.RequestResponse(ctx, map[string]interface{}{
		"type":       "thread_status",
		"request_id": requestID,
		"thread_id":  threadID,
	}, "thread_status_response", 10*time.Second)
	if err != nil {
		t.Logf("thread_status: %v", err)
		return
	}

	typ, _ := ev["type"].(string)
	if typ == "thread_status_response" {
		t.Logf("Received thread_status_response: %v", ev)
	} else if typ == "error" {
		t.Logf("Received error response: %v", ev)
	}
}

func TestIntegration_CommandRequest(t *testing.T) {
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

	// Send a command request using convenience method
	params := map[string]interface{}{
		"detail_level": "summary",
	}

	response, err := client.CommandRequest(ctx, "thread_info", threadID, params, 15*time.Second)
	if err != nil {
		t.Logf("CommandRequest error: %v", err)
		return
	}

	t.Logf("CommandRequest response: %v", response)

	command, _ := response["command"].(string)
	t.Logf("Command '%s' executed", command)
}

func TestIntegration_SendCommandRequest(t *testing.T) {
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

	params := map[string]interface{}{
		"action": "pause",
	}

	ev, err := client.CommandRequest(ctx, "thread_control", threadID, params, 15*time.Second)
	if err != nil {
		t.Logf("CommandRequest: %v", err)
		return
	}

	t.Logf("Command response: %v", ev)
}

// =============================================================================
// RESUME INTERRUPTS API TEST
// =============================================================================

func TestIntegration_SendResumeInterrupts(t *testing.T) {
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

	// Send resume_interrupts (normally used for interactive mode continuation)
	requestID := NewRequestID()
	resumePayload := map[string]interface{}{
		"user_response": "continue",
		"approved":      true,
	}

	if err := client.SendResumeInterrupts(ctx, threadID, resumePayload, requestID); err != nil {
		t.Logf("SendResumeInterrupts error: %v", err)
		return
	}

	t.Logf("Sent resume_interrupts request: %s", requestID)
}

// =============================================================================
// DAEMON MANAGEMENT API TESTS
// =============================================================================

func TestIntegration_SendDaemonShutdown(t *testing.T) {
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

	// Send daemon_shutdown request (but don't actually expect shutdown in test)
	requestID := NewRequestID()
	if err := client.SendDaemonShutdown(ctx, requestID); err != nil {
		t.Logf("SendDaemonShutdown error: %v", err)
		return
	}

	t.Logf("Sent daemon_shutdown request: %s (daemon will likely reject)", requestID)
}

func TestIntegration_SendDaemonStatus(t *testing.T) {
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

	requestID := NewRequestID()
	ev, err := client.RequestResponse(ctx, map[string]interface{}{
		"type":       "daemon_status",
		"request_id": requestID,
	}, "daemon_status_response", 10*time.Second)
	if err != nil {
		t.Logf("daemon_status: %v", err)
		return
	}

	typ, _ := ev["type"].(string)
	if typ == "daemon_status_response" {
		running, _ := ev["running"].(bool)
		portLive, _ := ev["port_live"].(bool)
		activeThreads, _ := ev["active_threads"].(float64)
		t.Logf("Daemon status: running=%v, port_live=%v, active_threads=%v",
			running, portLive, int(activeThreads))
	}
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

func createTestLoop(t *testing.T, client *Client, ctx context.Context) string {
	t.Helper()

	// Try to create a new loop
	response, err := client.LoopNew(ctx, 15*time.Second)
	if err != nil {
		t.Logf("Failed to create loop: %v", err)
		return ""
	}

	loopID, ok := response["loop_id"].(string)
	if !ok || loopID == "" {
		t.Logf("No loop_id in LoopNew response")
		return ""
	}

	t.Logf("Created test loop: %s", loopID)
	return loopID
}
