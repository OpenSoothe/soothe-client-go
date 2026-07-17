package soothe

import (
	"context"
	"testing"
	"time"
)

func TestCommandClient_AutopilotStatus(t *testing.T) {
	ts := newTestServer(testRequestResponseHandler)
	defer ts.Close()

	cc := NewCommandClient(wsURL(ts.URL), 5*time.Second)
	ctx := context.Background()

	result, err := cc.AutopilotStatus(ctx)
	if err != nil {
		t.Fatalf("AutopilotStatus: %v", err)
	}
	if running, _ := result["running"].(bool); !running {
		t.Fatalf("expected running=true, got %#v", result)
	}
}

func TestCommandClient_AutopilotSubmit(t *testing.T) {
	ts := newTestServer(testRequestResponseHandler)
	defer ts.Close()

	cc := NewCommandClient(wsURL(ts.URL), 5*time.Second)
	ctx := context.Background()

	result, err := cc.AutopilotSubmit(ctx, "deploy the app", 80, "/tmp")
	if err != nil {
		t.Fatalf("AutopilotSubmit: %v", err)
	}
	if goalID, _ := result["goal_id"].(string); goalID != "goal-1" {
		t.Fatalf("expected goal_id=goal-1, got %#v", result)
	}
}

func TestClient_AutopilotGoalRPCs(t *testing.T) {
	ts := newTestServer(testRequestResponseHandler)
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	timeout := 5 * time.Second

	if _, err := client.AutopilotListGoals(ctx, timeout); err != nil {
		t.Fatalf("AutopilotListGoals: %v", err)
	}
	if _, err := client.AutopilotGetGoal(ctx, "g1", timeout); err != nil {
		t.Fatalf("AutopilotGetGoal: %v", err)
	}
	if _, err := client.AutopilotCancelGoal(ctx, "g1", timeout); err != nil {
		t.Fatalf("AutopilotCancelGoal: %v", err)
	}
	if _, err := client.AutopilotCancelAll(ctx, timeout); err != nil {
		t.Fatalf("AutopilotCancelAll: %v", err)
	}
	if _, err := client.AutopilotWake(ctx, timeout); err != nil {
		t.Fatalf("AutopilotWake: %v", err)
	}
	if _, err := client.AutopilotDream(ctx, timeout); err != nil {
		t.Fatalf("AutopilotDream: %v", err)
	}
	if _, err := client.AutopilotResume(ctx, "g1", timeout); err != nil {
		t.Fatalf("AutopilotResume: %v", err)
	}
	if _, err := client.AutopilotListJobs(ctx, timeout); err != nil {
		t.Fatalf("AutopilotListJobs: %v", err)
	}
	if _, err := client.AutopilotGetJob(ctx, "j1", timeout); err != nil {
		t.Fatalf("AutopilotGetJob: %v", err)
	}
}

func TestClient_AutopilotSubmitRequiresDescription(t *testing.T) {
	client := NewClient("ws://127.0.0.1:1", nil)
	_, err := client.AutopilotSubmit(context.Background(), "", 50, "", time.Second)
	if err == nil {
		t.Fatal("expected error for empty description")
	}
}
