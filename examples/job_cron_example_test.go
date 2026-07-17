package soothe_test

import (
	"context"
	"fmt"
	"time"

	soothe "github.com/mirasoth/soothe-client-go"
)

// Example_jobLifecycle demonstrates the full autopilot job lifecycle:
// create -> status -> pause -> resume -> cancel, with DAG visualization.
func Example_jobLifecycle() {
	md := NewMockDaemon(nil)
	defer md.Close()

	client := soothe.NewClient(md.URL, nil)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	// Start the event reader so responses are multiplexed.
	_, err := client.ReceiveMessages(ctx)
	if err != nil {
		fmt.Printf("ReceiveMessages error: %v\n", err)
		return
	}

	// 1. Create a job with a root goal and workspace.
	createResp, err := client.JobCreate(ctx, "Build a REST API with tests", "/tmp/workspace", 15*time.Second)
	if err != nil {
		fmt.Printf("JobCreate error: %v\n", err)
		return
	}
	jobID, _ := createResp["job_id"].(string)
	fmt.Printf("Created job: %s\n", jobID)

	// 2. Query job status (goal status, counts, assigned workers).
	status, err := client.JobStatus(ctx, jobID, 15*time.Second)
	if err != nil {
		fmt.Printf("JobStatus error: %v\n", err)
	} else {
		fmt.Printf("Job status: %v\n", status)
	}

	// 3. Get the GoalEngine DAG snapshot for visualization.
	dag, err := client.JobDag(ctx, jobID, 15*time.Second)
	if err != nil {
		fmt.Printf("JobDag error: %v\n", err)
	} else {
		fmt.Printf("Job DAG: %v\n", dag)
	}

	// 4. Pause goal execution.
	if _, err := client.JobPause(ctx, jobID, 15*time.Second); err != nil {
		fmt.Printf("JobPause error: %v\n", err)
	} else {
		fmt.Println("JobPause: ok")
	}

	// 5. Resume paused goal execution.
	if _, err := client.JobResume(ctx, jobID, 15*time.Second); err != nil {
		fmt.Printf("JobResume error: %v\n", err)
	} else {
		fmt.Println("JobResume: ok")
	}

	// 6. Send user guidance to the job's root goal.
	if _, err := client.JobGuidance(ctx, jobID, "Prioritize authentication and security", "", 30*time.Second); err != nil {
		fmt.Printf("JobGuidance error: %v\n", err)
	} else {
		fmt.Println("JobGuidance: ok")
	}

	// 7. Cancel the job.
	if _, err := client.JobCancel(ctx, jobID, 15*time.Second); err != nil {
		fmt.Printf("JobCancel error: %v\n", err)
	} else {
		fmt.Println("JobCancel: ok")
	}
	// Output:
	// Created job: job-1
	// Job status: map[completed:0 goal_status:in_progress job_id:job-1 total:5 workers:3]
	// Job DAG: map[dag:root_goal
	//   ├── subtask_1
	//   ├── subtask_2
	//   └── subtask_3 job_id:job-1]
	// JobPause: ok
	// JobResume: ok
	// JobGuidance: ok
	// JobCancel: ok
}

// Example_sendJobMethods uses the low-level SendJob* methods (fire-and-forget
// request envelopes) when the event reader handles responses.
func Example_sendJobMethods() {
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

	jobID := "existing-job-id"

	// Fire-and-forget job operations (responses arrive on the event channel).
	if err := client.SendJobCreate(ctx, "Build a REST API", "/tmp/workspace"); err != nil {
		fmt.Printf("SendJobCreate error: %v\n", err)
	}
	if err := client.SendJobStatus(ctx, jobID); err != nil {
		fmt.Printf("SendJobStatus error: %v\n", err)
	}
	if err := client.SendJobPause(ctx, jobID); err != nil {
		fmt.Printf("SendJobPause error: %v\n", err)
	}
	if err := client.SendJobResume(ctx, jobID); err != nil {
		fmt.Printf("SendJobResume error: %v\n", err)
	}
	if err := client.SendJobCancel(ctx, jobID); err != nil {
		fmt.Printf("SendJobCancel error: %v\n", err)
	}
	if err := client.SendJobDag(ctx, jobID); err != nil {
		fmt.Printf("SendJobDag error: %v\n", err)
	}
	fmt.Println("Job requests sent")
	// Output:
	// Job requests sent
}

// Example_autopilotSubscribe subscribes to autopilot worker events.
// AutopilotSubscribe returns a subscription ID for later unsubscription.
func Example_autopilotSubscribe() {
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

	// Subscribe to autopilot worker lifecycle events.
	subID, err := client.AutopilotSubscribe(ctx, 15*time.Second)
	if err != nil {
		fmt.Printf("AutopilotSubscribe error: %v\n", err)
		return
	}
	fmt.Printf("Subscribed to autopilot events (id length: %d)\n", len(subID))

	// Consume autopilot events...
	go func() {
		for msg := range eventCh {
			if msg == nil {
				return
			}
			// Look for soothe.system.autopilot.* events...
			_ = msg
		}
	}()

	// Unsubscribe when done (blocking, waits for daemon response).
	resp, err := client.AutopilotUnsubscribe(ctx, subID, 15*time.Second)
	if err != nil {
		fmt.Printf("AutopilotUnsubscribe error: %v\n", err)
	} else {
		fmt.Printf("Unsubscribed: %v\n", resp)
	}
	// Output:
	// Subscribed to autopilot events (id length: 36)
	// Unsubscribed: map[status:unsubscribed]
}

// Example_cronLifecycle demonstrates scheduled job management via the cron API.
func Example_cronLifecycle() {
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

	// 1. Create a scheduled job from natural language.
	addResp, err := client.CronAdd(ctx, "Every morning at 9am, summarize overnight alerts", 5, 30*time.Second)
	if err != nil {
		fmt.Printf("CronAdd error: %v\n", err)
		return
	}
	jobID, _ := addResp["job_id"].(string)
	fmt.Printf("Created cron job: %s\n", jobID)

	// 2. List all scheduled jobs (optionally filter by status).
	list, err := client.CronList(ctx, "active", 15*time.Second)
	if err != nil {
		fmt.Printf("CronList error: %v\n", err)
	} else {
		fmt.Printf("Cron jobs: %v\n", list)
	}

	// 3. Show details for a specific cron job.
	show, err := client.CronShow(ctx, jobID, 15*time.Second)
	if err != nil {
		fmt.Printf("CronShow error: %v\n", err)
	} else {
		fmt.Printf("Cron details: %v\n", show)
	}

	// 4. Cancel the scheduled job.
	if _, err := client.CronCancel(ctx, jobID, 15*time.Second); err != nil {
		fmt.Printf("CronCancel error: %v\n", err)
	} else {
		fmt.Println("CronCancel: ok")
	}
	// Output:
	// Created cron job: cron-1
	// Cron jobs: map[jobs:[map[job_id:cron-1 status:active text:morning summary]] total:1]
	// Cron details: map[job_id:cron-1 status:active text:morning summary]
	// CronCancel: ok
}

// Example_sendCronMethods uses the low-level SendCron* methods (fire-and-forget
// request envelopes) when the event reader handles responses.
func Example_sendCronMethods() {
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

	// Fire-and-forget cron operations.
	if err := client.SendCronAdd(ctx, "Every morning at 9am, summarize overnight alerts", 5); err != nil {
		fmt.Printf("SendCronAdd error: %v\n", err)
	}
	if err := client.SendCronList(ctx, "active"); err != nil {
		fmt.Printf("SendCronList error: %v\n", err)
	}
	if err := client.SendCronShow(ctx, "cron-1"); err != nil {
		fmt.Printf("SendCronShow error: %v\n", err)
	}
	if err := client.SendCronCancel(ctx, "cron-1"); err != nil {
		fmt.Printf("SendCronCancel error: %v\n", err)
	}
	fmt.Println("Cron requests sent")
	// Output:
	// Cron requests sent
}
