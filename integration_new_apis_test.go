package soothe

import (
	"context"
	"testing"
	"time"
)

// =============================================================================
// Job IPC Integration Tests
// =============================================================================

func TestIntegration_JobCreate(t *testing.T) {
	skipIfNoDaemon(t)

	// Reduced from 60s to 35s for faster test execution
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	// Create an autopilot job with reduced timeout - 15s instead of 30s
	response, err := client.JobCreate(ctx, "Write a simple hello world program", "", 15*time.Second)
	if err != nil {
		t.Logf("JobCreate error: %v", err)
		return
	}

	jobID, ok := response["job_id"].(string)
	if !ok || jobID == "" {
		t.Logf("No job_id in JobCreate response: %v", response)
		return
	}

	t.Logf("Created autopilot job: %s", jobID)
	t.Logf("JobCreate response: %v", response)

	// Clean up: cancel the job - reduced from 10s to 5s
	_, _ = client.JobCancel(ctx, jobID, 5*time.Second)
}

func TestIntegration_JobStatus(t *testing.T) {
	skipIfNoDaemon(t)

	// Reduced from 60s to 35s for faster test execution
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	// First create a job
	createResp, err := client.JobCreate(ctx, "List files in current directory", "", 30*time.Second)
	if err != nil {
		t.Logf("JobCreate error: %v", err)
		return
	}

	jobID, ok := createResp["job_id"].(string)
	if !ok || jobID == "" {
		t.Skip("No job_id in JobCreate response")
	}

	t.Logf("Created job for status test: %s", jobID)

	// Query job status
	statusResp, err := client.JobStatus(ctx, jobID, 15*time.Second)
	if err != nil {
		t.Logf("JobStatus error: %v", err)
	} else {
		t.Logf("JobStatus response: %v", statusResp)
	}

	// Clean up
	_, _ = client.JobCancel(ctx, jobID, 10*time.Second)
}

func TestIntegration_JobPauseResume(t *testing.T) {
	skipIfNoDaemon(t)

	ctx := context.Background()
	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	// Create a job
	createResp, err := client.JobCreate(ctx, "Count to 10 slowly", "", 30*time.Second)
	if err != nil {
		t.Logf("JobCreate error: %v", err)
		return
	}

	jobID, ok := createResp["job_id"].(string)
	if !ok || jobID == "" {
		t.Skip("No job_id in JobCreate response")
	}

	t.Logf("Created job for pause/resume test: %s", jobID)

	// Pause the job
	pauseResp, err := client.JobPause(ctx, jobID, 15*time.Second)
	if err != nil {
		t.Logf("JobPause error: %v", err)
	} else {
		t.Logf("JobPause response: %v", pauseResp)
	}

	// Resume the job
	resumeResp, err := client.JobResume(ctx, jobID, 15*time.Second)
	if err != nil {
		t.Logf("JobResume error: %v", err)
	} else {
		t.Logf("JobResume response: %v", resumeResp)
	}

	// Clean up
	_, _ = client.JobCancel(ctx, jobID, 10*time.Second)
}

func TestIntegration_JobCancel(t *testing.T) {
	skipIfNoDaemon(t)

	// Use test-level timeout to prevent hanging if daemon doesn't respond
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	// Create a job with reduced timeout
	createResp, err := client.JobCreate(ctx, "Long running task that will be cancelled", "", 20*time.Second)
	if err != nil {
		t.Logf("JobCreate error: %v", err)
		return
	}

	jobID, ok := createResp["job_id"].(string)
	if !ok || jobID == "" {
		t.Skip("No job_id in JobCreate response")
	}

	t.Logf("Created job for cancel test: %s", jobID)

	// Cancel the job with reduced timeout
	cancelResp, err := client.JobCancel(ctx, jobID, 10*time.Second)
	if err != nil {
		t.Logf("JobCancel error: %v", err)
		// Check if context deadline was exceeded
		if ctx.Err() == context.DeadlineExceeded {
			t.Log("Test context deadline exceeded - daemon may not be responding")
		}
	} else {
		t.Logf("JobCancel response: %v", cancelResp)
		success, _ := cancelResp["success"].(bool)
		if success {
			t.Logf("Successfully cancelled job %s", jobID)
		}
	}
}

func TestIntegration_JobDag(t *testing.T) {
	skipIfNoDaemon(t)

	// Use test-level timeout to prevent hanging - reduced from 60s to 35s
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	// Create a job with reduced timeout - 15s instead of 30s
	createResp, err := client.JobCreate(ctx, "Simple task for DAG visualization", "", 15*time.Second)
	if err != nil {
		t.Logf("JobCreate error: %v", err)
		return
	}

	jobID, ok := createResp["job_id"].(string)
	if !ok || jobID == "" {
		t.Skip("No job_id in JobCreate response")
	}

	t.Logf("Created job for DAG test: %s", jobID)

	// Request DAG visualization with reduced timeout - 8s instead of 15s
	dagResp, err := client.JobDag(ctx, jobID, 8*time.Second)
	if err != nil {
		t.Logf("JobDag error: %v", err)
	} else {
		t.Logf("JobDag response: %v", dagResp)
	}

	// Clean up with reduced timeout - 5s instead of 10s
	_, _ = client.JobCancel(ctx, jobID, 5*time.Second)
}

func TestIntegration_JobGuidance(t *testing.T) {
	skipIfNoDaemon(t)

	// Use test-level timeout - reduced from 60s to 35s
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	// Create a job with reduced timeout - 15s instead of 30s
	createResp, err := client.JobCreate(ctx, "A task to receive guidance", "", 15*time.Second)
	if err != nil {
		t.Logf("JobCreate error: %v", err)
		return
	}

	jobID, ok := createResp["job_id"].(string)
	if !ok || jobID == "" {
		t.Skip("No job_id in JobCreate response")
	}

	t.Logf("Created job for guidance test: %s", jobID)

	// Send guidance to the job with reduced timeout - 10s instead of 30s
	guidanceResp, err := client.JobGuidance(ctx, jobID, "Please focus on quality over speed", "", 10*time.Second)
	if err != nil {
		t.Logf("JobGuidance error: %v", err)
	} else {
		t.Logf("JobGuidance response: %v", guidanceResp)
	}

	// Clean up with reduced timeout - 5s instead of 10s
	_, _ = client.JobCancel(ctx, jobID, 5*time.Second)
}

func TestIntegration_AutopilotSubscribe(t *testing.T) {
	skipIfNoDaemon(t)

	// Use test-level timeout - reduced from 60s to 30s
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	// Subscribe to autopilot worker events - reduced from 15s to 8s
	subID, err := client.AutopilotSubscribe(ctx, 8*time.Second)
	if err != nil {
		t.Logf("AutopilotSubscribe error: %v", err)
	} else {
		t.Logf("AutopilotSubscribe subscription id: %s", subID)
	}

	// Unsubscribe — use the subscription id returned by subscribe (the daemon
	// infers autopilot unsubscribe from the unsubscribe envelope with no loop_id).
	// Reduced from 15s to 8s
	unsubResp, err := client.AutopilotUnsubscribe(ctx, subID, 8*time.Second)
	if err != nil {
		t.Logf("AutopilotUnsubscribe error: %v", err)
	} else {
		t.Logf("AutopilotUnsubscribe response: %v", unsubResp)
	}
}

// =============================================================================
// Additional Loop Methods Integration Tests
// =============================================================================

func TestIntegration_LoopMessages(t *testing.T) {
	skipIfNoDaemon(t)

	// Use test-level timeout to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	// Create a loop
	newResp, err := client.LoopNew(ctx, 15*time.Second)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	loopID, ok := newResp["loop_id"].(string)
	if !ok || loopID == "" {
		t.Skip("No loop_id in LoopNew response")
	}
	t.Logf("Created test loop: %s", loopID)

	// Request persisted messages
	response, err := client.LoopMessages(ctx, loopID, 10, 0, false, 15*time.Second)
	if err != nil {
		t.Logf("LoopMessages error: %v", err)
		return
	}

	t.Logf("LoopMessages response: %v", response)
}

func TestIntegration_LoopStateGet(t *testing.T) {
	skipIfNoDaemon(t)

	// Use test-level timeout to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	// Create a loop
	newResp, err := client.LoopNew(ctx, 15*time.Second)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	loopID, ok := newResp["loop_id"].(string)
	if !ok || loopID == "" {
		t.Skip("No loop_id in LoopNew response")
	}
	t.Logf("Created test loop: %s", loopID)

	// Request loop state
	response, err := client.LoopStateGet(ctx, loopID, 15*time.Second)
	if err != nil {
		t.Logf("LoopStateGet error: %v", err)
		return
	}

	t.Logf("LoopStateGet response: %v", response)
}

func TestIntegration_MCPStatus(t *testing.T) {
	skipIfNoDaemon(t)

	// Use test-level timeout to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	// Request MCP server status
	response, err := client.MCPStatus(ctx, 15*time.Second)
	if err != nil {
		t.Logf("MCPStatus error: %v", err)
		return
	}

	t.Logf("MCPStatus response: %v", response)

	// Check for expected fields
	if servers, ok := response["servers"]; ok {
		t.Logf("MCP servers: %v", servers)
	}
}

// =============================================================================
// Send Methods Integration Tests
// =============================================================================

func TestIntegration_SendLoopMessages(t *testing.T) {
	skipIfNoDaemon(t)

	// Use test-level timeout to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	// Create a loop
	newResp, err := client.LoopNew(ctx, 15*time.Second)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	loopID, ok := newResp["loop_id"].(string)
	if !ok || loopID == "" {
		t.Skip("No loop_id in LoopNew response")
	}
	t.Logf("Created test loop: %s", loopID)

	// Send loop_messages request (fire-and-forget)
	requestID := NewRequestID()
	if err := client.SendLoopMessages(ctx, loopID, 10, 0, false, requestID); err != nil {
		t.Logf("SendLoopMessages error: %v", err)
	}

	t.Logf("Sent loop_messages request: %s", requestID)
}

func TestIntegration_SendLoopStateGet(t *testing.T) {
	skipIfNoDaemon(t)

	// Use test-level timeout to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	// Create a loop
	newResp, err := client.LoopNew(ctx, 15*time.Second)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	loopID, ok := newResp["loop_id"].(string)
	if !ok || loopID == "" {
		t.Skip("No loop_id in LoopNew response")
	}
	t.Logf("Created test loop: %s", loopID)

	// Send loop_state_get request (fire-and-forget)
	requestID := NewRequestID()
	if err := client.SendLoopStateGet(ctx, loopID, requestID); err != nil {
		t.Logf("SendLoopStateGet error: %v", err)
	}

	t.Logf("Sent loop_state_get request: %s", requestID)
}

func TestIntegration_SendMCPStatus(t *testing.T) {
	skipIfNoDaemon(t)

	// Use test-level timeout to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	// Send mcp_status request (fire-and-forget)
	requestID := NewRequestID()
	if err := client.SendMCPStatus(ctx, requestID); err != nil {
		t.Logf("SendMCPStatus error: %v", err)
	}

	t.Logf("Sent mcp_status request: %s", requestID)
}

func TestIntegration_LoopHistoryFetch(t *testing.T) {
	skipIfNoDaemon(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	// Create a loop
	newResp, err := client.LoopNew(ctx, 15*time.Second)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	loopID, ok := newResp["loop_id"].(string)
	if !ok || loopID == "" {
		t.Skip("No loop_id in LoopNew response")
	}
	t.Logf("Created test loop: %s", loopID)

	// Request loop history
	historyResp, err := client.LoopHistoryFetch(ctx, loopID, 15*time.Second)
	if err != nil {
		t.Logf("LoopHistoryFetch error: %v", err)
	} else {
		t.Logf("LoopHistoryFetch response: %v", historyResp)
	}
}

func TestIntegration_LoopExecutionStateFetch(t *testing.T) {
	skipIfNoDaemon(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	client := NewClient("ws://localhost:8765", integrationTestConfig())

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	if _, err := client.WaitForDaemonReady(10 * time.Second); err != nil {
		t.Fatalf("Daemon not ready: %v", err)
	}

	// Create a loop
	newResp, err := client.LoopNew(ctx, 15*time.Second)
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	loopID, ok := newResp["loop_id"].(string)
	if !ok || loopID == "" {
		t.Skip("No loop_id in LoopNew response")
	}
	t.Logf("Created test loop: %s", loopID)

	// Request execution state snapshot
	stateResp, err := client.LoopExecutionStateFetch(ctx, loopID, 15*time.Second)
	if err != nil {
		t.Logf("LoopExecutionStateFetch error: %v", err)
	} else {
		t.Logf("LoopExecutionStateFetch response: %v", stateResp)
	}
}
