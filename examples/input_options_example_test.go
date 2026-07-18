package soothe_test

import (
	"context"
	"fmt"
	"time"

	soothe "github.com/mirasoth/soothe-client-go"
)

// Example_sendInputBasic sends a simple text input to a loop using the
// functional-options pattern. WithLoopID is always required.
func Example_sendInputBasic() {
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

	// SendInput sends a loop_input notification (fire-and-forget).
	// WithLoopID is mandatory; other options are optional.
	err := client.SendInput(ctx, "Explain how goroutines work in Go", soothe.WithLoopID(loopID))
	if err != nil {
		fmt.Printf("SendInput error: %v\n", err)
	} else {
		fmt.Println("Input sent")
	}
	// Output:
	// Input sent
}

// Example_sendInputAutonomous enables autonomous mode with a max-iterations cap.
// The daemon runs the agent graph without streaming intermediate steps.
func Example_sendInputAutonomous() {
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
	maxIter := 10

	err := client.SendInput(ctx, "Refactor the auth module for better testability",
		soothe.WithLoopID(loopID),
		soothe.WithAutonomous(&maxIter),
	)
	if err != nil {
		fmt.Printf("SendInput error: %v\n", err)
	} else {
		fmt.Println("Autonomous input sent")
	}
	// Output:
	// Autonomous input sent
}

// Example_sendInputWithModel overrides the provider:model for this turn
// and passes extra model parameters.
func Example_sendInputWithModel() {
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

	err := client.SendInput(ctx, "Summarize the quarterly report",
		soothe.WithLoopID(loopID),
		soothe.WithModel("openai:gpt-4o"),
		soothe.WithModelParams(map[string]interface{}{
			"temperature": 0.3,
			"max_tokens":  2000,
		}),
	)
	if err != nil {
		fmt.Printf("SendInput error: %v\n", err)
	} else {
		fmt.Println("Input with model sent")
	}
	// Output:
	// Input with model sent
}

// Example_sendInputWithAttachments attaches image data (base64-encoded) and
// uses the image_to_text intent hint for OCR-like tasks.
func Example_sendInputWithAttachments() {
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

	// Each attachment is {mime_type, data(base64)}.
	attachments := []map[string]interface{}{
		{
			"mime_type": "image/png",
			"data":      "iVBORw0KGgoAAAANSUhEUg==", // truncated base64
		},
	}

	err := client.SendInput(ctx, "Describe what's in this image",
		soothe.WithLoopID(loopID),
		soothe.WithAttachments(attachments),
		soothe.WithIntentHint(soothe.IntentHintImageToText),
	)
	if err != nil {
		fmt.Printf("SendInput error: %v\n", err)
	} else {
		fmt.Println("Input with attachments sent")
	}
	// Output:
	// Input with attachments sent
}

// Example_sendInputStructuredOutput requests JSON Schema-structured output
// using the text_completion intent hint.
func Example_sendInputStructuredOutput() {
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

	// Define a JSON Schema for structured output.
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"summary": map[string]interface{}{"type": "string"},
			"score":   map[string]interface{}{"type": "number"},
		},
		"required": []string{"summary", "score"},
	}

	err := client.SendInput(ctx, "Review this pull request",
		soothe.WithLoopID(loopID),
		soothe.WithIntentHint(soothe.IntentHintTextCompletion),
		soothe.WithResponseSchema(schema),
		soothe.WithResponseSchemaName("pr_review"),
		soothe.WithResponseSchemaStrict(true),
	)
	if err != nil {
		fmt.Printf("SendInput error: %v\n", err)
	} else {
		fmt.Println("Structured output input sent")
	}
	// Output:
	// Structured output input sent
}

// Example_sendInputWithSubagent routes the input to a preferred subagent
// (e.g. "explorer" for codebase exploration, "deep_research" for research).
func Example_sendInputWithSubagent() {
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

	err := client.SendInput(ctx, "Explore the database migration patterns in this codebase",
		soothe.WithLoopID(loopID),
		soothe.WithSubagent("explorer"),
	)
	if err != nil {
		fmt.Printf("SendInput error: %v\n", err)
	} else {
		fmt.Println("Input with subagent sent")
	}
	// Output:
	// Input with subagent sent
}

// Example_sendInputClarification shows answering a pending clarification
// interrupt from the daemon.
func Example_sendInputClarification() {
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

	// Answer a single pending clarification question.
	err := client.SendInput(ctx, "Use PostgreSQL",
		soothe.WithLoopID(loopID),
		soothe.WithClarificationAnswer(),
	)
	if err != nil {
		fmt.Printf("SendInput error: %v\n", err)
	}

	// Answer multiple questions in a multi-question clarification.
	err = client.SendInput(ctx, "",
		soothe.WithLoopID(loopID),
		soothe.WithClarificationAnswers([]string{"PostgreSQL", "us-east-1", "3"}),
	)
	if err != nil {
		fmt.Printf("SendInput error: %v\n", err)
	}

	// Set clarification relay mode.
	err = client.SendInput(ctx, "",
		soothe.WithLoopID(loopID),
		soothe.WithClarificationMode("manual"),
	)
	if err != nil {
		fmt.Printf("SendInput error: %v\n", err)
	} else {
		fmt.Println("Clarification answers sent")
	}
	// Output:
	// Clarification answers sent
}

// Example_sendLoopInput uses the lower-level SendLoopInput method, which sends
// a minimal loop_input notification without the InputOption builder.
func Example_sendLoopInput() {
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

	// SendLoopInput: minimal loop_input with just loop_id and content.
	if err := client.SendLoopInput(ctx, loopID, "What is the meaning of life?"); err != nil {
		fmt.Printf("SendLoopInput error: %v\n", err)
	}

	// LoopInput (blocking): sends loop_input and waits for the response.
	resp, err := client.LoopInput(ctx, loopID, "What is the meaning of life?", 60*time.Second)
	if err != nil {
		fmt.Printf("LoopInput error: %v\n", err)
	} else {
		fmt.Printf("Response: %v\n", resp)
	}
	// Output:
	// Response: map[accepted:true loop_id:existing-loop-id]
}
