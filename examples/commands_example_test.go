package soothe_test

import (
	"context"
	"fmt"
	"time"

	soothe "github.com/mirasoth/soothe-client-go"
)

// Example_commandRequest sends a structured RPC command and waits
// for the response. This is the generic command wrapper.
func Example_commandRequest() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	loopID := "existing-loop-id"

	// CommandRequest: generic command with optional params.
	resp, err := client.CommandRequest(ctx, "memory", loopID, nil, 10*time.Second)
	if err != nil {
		fmt.Printf("CommandRequest error: %v\n", err)
		return
	}
	fmt.Printf("Command response: %v\n", resp)
	// Output:
	// Command response: map[context_window:128000 limit:128000 token_usage:1500 used:1500]
}

// Example_commandClear cancels the current query and clears conversation
// history for a loop.
func Example_commandClear() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	loopID := "existing-loop-id"

	// Clear conversation history.
	resp, err := client.CommandClear(ctx, loopID, 10*time.Second)
	if err != nil {
		fmt.Printf("CommandClear error: %v\n", err)
		return
	}
	fmt.Printf("Cleared: %v\n", resp)
	// Output:
	// Cleared: map[status:cleared]
}

// Example_commandCancel cancels the currently running query for a loop.
// This sends command_request{command:"cancel"} to the daemon.
func Example_commandCancel() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	loopID := "existing-loop-id"

	resp, err := client.CommandCancel(ctx, loopID, 10*time.Second)
	if err != nil {
		fmt.Printf("CommandCancel error: %v\n", err)
		return
	}
	fmt.Printf("Cancelled: %v\n", resp)
	// Output:
	// Cancelled: map[status:cleared]
}

// Example_commandMemory queries memory stats for a loop (token usage, context
// window state, etc.).
func Example_commandMemory() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	loopID := "existing-loop-id"

	resp, err := client.CommandMemory(ctx, loopID, 10*time.Second)
	if err != nil {
		fmt.Printf("CommandMemory error: %v\n", err)
		return
	}
	fmt.Printf("Memory: %v\n", resp)
	// Output:
	// Memory: map[context_window:128000 limit:128000 token_usage:1500 used:1500]
}

// Example_commandHistory queries the input history for a loop.
func Example_commandHistory() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	loopID := "existing-loop-id"

	resp, err := client.CommandHistory(ctx, loopID, 10*time.Second)
	if err != nil {
		fmt.Printf("CommandHistory error: %v\n", err)
		return
	}
	fmt.Printf("History: %v\n", resp)
	// Output:
	// History: map[history:[What is a goroutine? How do channels work? Explain the sync package]]
}

// Example_commandPlan queries the current plan for a loop.
func Example_commandPlan() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	loopID := "existing-loop-id"

	resp, err := client.CommandPlan(ctx, loopID, 10*time.Second)
	if err != nil {
		fmt.Printf("CommandPlan error: %v\n", err)
		return
	}
	fmt.Printf("Plan: %v\n", resp)
	// Output:
	// Plan: map[plan:1. Understand the question
	// 2. Gather context
	// 3. Synthesize answer]
}

// Example_commandDetachExit demonstrates lifecycle commands: detach (keeps
// loop running) and exit (stops the loop).
func Example_commandDetachExit() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	loopID := "existing-loop-id"

	// Detach: marks the loop as detached but keeps it running server-side.
	detachResp, err := client.CommandDetach(ctx, loopID, 10*time.Second)
	if err != nil {
		fmt.Printf("CommandDetach error: %v\n", err)
	} else {
		fmt.Printf("Detach: %v\n", detachResp)
	}

	// Exit: stops the loop and marks it for exit.
	exitResp, err := client.CommandExit(ctx, loopID, 10*time.Second)
	if err != nil {
		fmt.Printf("CommandExit error: %v\n", err)
	} else {
		fmt.Printf("Exit: %v\n", exitResp)
	}

	// Quit is an alias for exit.
	quitResp, err := client.CommandQuit(ctx, loopID, 10*time.Second)
	if err != nil {
		fmt.Printf("CommandQuit error: %v\n", err)
	} else {
		fmt.Printf("Quit: %v\n", quitResp)
	}
	// Output:
	// Detach: map[status:detached]
	// Exit: map[status:exit]
	// Quit: map[status:exit]
}

// Example_commandPolicyConfig shows global (non-loop-scoped) commands.
func Example_commandPolicyConfig() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	// Policy: query the daemon's policy profile (no loop_id needed).
	policy, err := client.CommandPolicy(ctx, 10*time.Second)
	if err != nil {
		fmt.Printf("CommandPolicy error: %v\n", err)
	} else {
		fmt.Printf("Policy: %v\n", policy)
	}

	// Config: query daemon configuration.
	config, err := client.CommandConfig(ctx, 10*time.Second)
	if err != nil {
		fmt.Printf("CommandConfig error: %v\n", err)
	} else {
		fmt.Printf("Config: %v\n", config)
	}
	// Output:
	// Policy: map[max_turns:20 policy:default]
	// Config: map[models:map[default:openai:gpt-4o]]
}

// Example_commandReview queries the conversation history (review) for a loop.
func Example_commandReview() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	loopID := "existing-loop-id"

	resp, err := client.CommandReview(ctx, loopID, 10*time.Second)
	if err != nil {
		fmt.Printf("CommandReview error: %v\n", err)
		return
	}
	fmt.Printf("Review: %v\n", resp)
	// Output:
	// Review: map[review:The conversation covers Go concurrency topics.]
}

// Example_commandAutopilotDashboard shows the autopilot dashboard for a loop.
func Example_commandAutopilotDashboard() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	loopID := "existing-loop-id"

	resp, err := client.CommandAutopilotDashboard(ctx, loopID, 10*time.Second)
	if err != nil {
		fmt.Printf("CommandAutopilotDashboard error: %v\n", err)
		return
	}
	fmt.Printf("Dashboard: %v\n", resp)
	// Output:
	// Dashboard: map[active:1 completed:2 jobs:3 paused:0]
}

// Example_sendCommand sends a slash command as a fire-and-forget notification.
// Useful for commands that do not require a response.
func Example_sendCommand() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	// SendCommand sends a slash_command notification (no response expected).
	if err := client.SendCommand(ctx, "/clear"); err != nil {
		fmt.Printf("SendCommand error: %v\n", err)
	}

	// SendDetach notifies the daemon this client is leaving (loops keep running).
	if err := client.SendDetach(ctx); err != nil {
		fmt.Printf("SendDetach error: %v\n", err)
	}
	fmt.Println("Commands sent")
	// Output:
	// Commands sent
}
