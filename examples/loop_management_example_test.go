package soothe_test

import (
	"context"
	"fmt"
	"time"

	soothe "github.com/mirasoth/soothe-client-go"
)

// Example_loopLifecycle demonstrates the full loop lifecycle: create, subscribe,
// send input, query details, list, and delete. Uses an in-process mock daemon
// so the example runs without a live Soothe daemon.
func Example_loopLifecycle() {
	md := NewMockDaemon(nil)
	defer md.Close()

	cfg := soothe.DefaultConfig()
	client := soothe.NewClient(md.URL, cfg)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	// Start the event reader before subscribing (multiplexer-aware).
	eventCh, err := client.ReceiveMessages(ctx)
	if err != nil {
		fmt.Printf("ReceiveMessages error: %v\n", err)
		return
	}

	// Bootstrap a fresh loop (loop_new + loop_subscribe in one call).
	loopID, err := soothe.BootstrapLoopSession(ctx, client, "", "/tmp/workspace", nil)
	if err != nil {
		fmt.Printf("Bootstrap error: %v\n", err)
		return
	}
	fmt.Printf("Created loop: %s\n", loopID)

	// Send input as a fire-and-forget notification (does not block for response).
	if err := client.SendInput(ctx, "Analyze this codebase", soothe.WithLoopID(loopID)); err != nil {
		fmt.Printf("SendInput error: %v\n", err)
	}

	// Consume streaming events from the loop.
	go func() {
		for msg := range eventCh {
			if msg == nil {
				return
			}
		}
	}()

	// Query loop details with verbose output.
	details, err := client.LoopGet(ctx, loopID, true, 15*time.Second)
	if err != nil {
		fmt.Printf("LoopGet error: %v\n", err)
	} else {
		fmt.Printf("Loop details: %v\n", details)
	}

	// List all loops, limited to 10.
	loops, err := client.LoopList(ctx, nil, 10, 15*time.Second)
	if err != nil {
		fmt.Printf("LoopList error: %v\n", err)
	} else {
		fmt.Printf("Loops: %v\n", loops)
	}

	// Clean up the loop when done.
	if _, err := client.LoopDelete(ctx, loopID, 10*time.Second); err != nil {
		fmt.Printf("LoopDelete error: %v\n", err)
	}
	// Output:
	// Created loop: loop-1
	// Loop details: map[active:true loop_id:loop-1 state:idle]
	// Loops: map[loops:[map[loop_id:loop-1 state:idle] map[loop_id:loop-2 state:running]] total:2]
}

// Example_loopSubscribeUnsubscribe shows manual subscribe/detach using the
// request-response wrappers (LoopSubscribe / LoopDetach) and the lower-level
// Send methods (SendLoopSubscribe / SendLoopDetach).
func Example_loopSubscribeUnsubscribe() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
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

	loopID := "existing-loop-id"

	// Blocking subscribe: waits for the `next` confirmation frame.
	subResult, err := client.LoopSubscribe(ctx, loopID, "normal", 10*time.Second)
	if err != nil {
		fmt.Printf("Subscribe error: %v\n", err)
		return
	}
	fmt.Printf("Subscribed: %v\n", subResult)

	// Low-level alternative: SendLoopSubscribe sends the subscribe envelope
	// without waiting for confirmation (useful when the event reader handles it).
	if err := client.SendLoopSubscribe(ctx, loopID, "normal", "adaptive"); err != nil {
		fmt.Printf("SendLoopSubscribe error: %v\n", err)
	}

	// Detach from the loop (unsubscribe by subscription id).
	if _, err := client.LoopDetach(ctx, loopID, 10*time.Second); err != nil {
		fmt.Printf("Detach error: %v\n", err)
	}

	_ = eventCh
	// Output:
	// Subscribed: map[client_id:mock-client event:subscribed loop_id:existing-loop-id success:true]
}

// Example_loopMessagesState shows querying persisted conversation rows and
// LangGraph checkpoint state for a loop.
func Example_loopMessagesState() {
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

	// Fetch the last 50 conversation/activity rows, including events.
	msgs, err := client.LoopMessages(ctx, loopID, 50, 0, true, 15*time.Second)
	if err != nil {
		fmt.Printf("LoopMessages error: %v\n", err)
	} else {
		fmt.Printf("Messages: %v\n", msgs)
	}

	// Read LangGraph checkpoint channel values.
	state, err := client.LoopStateGet(ctx, loopID, 15*time.Second)
	if err != nil {
		fmt.Printf("LoopStateGet error: %v\n", err)
	} else {
		fmt.Printf("State: %v\n", state)
	}

	// Apply partial state update (e.g. inject a variable).
	updates := map[string]interface{}{"user_preferences": map[string]string{"lang": "en"}}
	if _, err := client.LoopStateUpdate(ctx, loopID, updates, "agent", 15*time.Second); err != nil {
		fmt.Printf("LoopStateUpdate error: %v\n", err)
	}
	// Output:
	// Messages: map[loop_id:existing-loop-id messages:[map[content:Hello role:user] map[content:Hi there role:assistant]] total:2]
	// State: map[loop_id:existing-loop-id state:map[messages:[]]]
}

// Example_loopTreeAndCards fetches the checkpoint tree and display card ledger
// for a loop.
func Example_loopTreeAndCards() {
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

	// Request the checkpoint tree in text format.
	tree, err := client.LoopTree(ctx, loopID, "text", 15*time.Second)
	if err != nil {
		fmt.Printf("LoopTree error: %v\n", err)
	} else {
		fmt.Printf("Tree: %v\n", tree)
	}

	// Fetch replayable history.
	history, err := soothe.FetchLoopHistory(ctx, client, loopID, 15*time.Second)
	if err != nil {
		fmt.Printf("FetchLoopHistory error: %v\n", err)
	} else {
		fmt.Printf("History: %v\n", history)
	}
	// Output:
	// Tree: map[format:text loop_id:existing-loop-id tree:root
	//   ├── checkpoint-1
	//   └── checkpoint-2]
	// History: map[history:[] loop_id:existing-loop-id replayable:true]
}

// Example_loopReattachResume shows reattaching to an existing loop after
// reconnect, including the BootstrapLoopSession resume parameter.
func Example_loopReattachResume() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	_, _ = client.ReceiveMessages(ctx)

	// BootstrapLoopSession with a non-empty resumeLoopID performs
	// loop_subscribe only (no loop_new).
	existingLoopID := "existing-loop-id"
	loopID, err := soothe.BootstrapLoopSession(ctx, client, existingLoopID, "/tmp/workspace", nil)
	if err != nil {
		fmt.Printf("Resume error: %v\n", err)
		return
	}
	fmt.Printf("Resumed loop: %s\n", loopID)

	// ReattachAndProbe: loop_reattach + re-subscribe + loop_get liveness probe.
	// Detects stale loops that accept the handshake but silently drop input.
	if err := client.ReattachAndProbe(ctx, existingLoopID); err != nil {
		fmt.Printf("Reattach error: %v\n", err)
	}
	// Output:
	// Resumed loop: existing-loop-id
}

// Example_loopPrune shows pruning old checkpoint branches for a loop.
func Example_loopPrune() {
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

	// Prune with 7-day retention, dry-run mode (preview only).
	result, err := client.LoopPrune(ctx, loopID, 7, true, 30*time.Second)
	if err != nil {
		fmt.Printf("LoopPrune error: %v\n", err)
	} else {
		fmt.Printf("Prune result: %v\n", result)
	}
	// Output:
	// Prune result: map[dry_run:true loop_id:existing-loop-id pruned:3 remaining:2]
}
