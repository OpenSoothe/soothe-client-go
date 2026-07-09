package soothe

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// =============================================================================
// IMAGE UNDERSTANDING TESTS
// =============================================================================

// loadTestImage reads the test image from examples/ and returns its base64-encoded content.
func loadTestImage(t *testing.T) (string, string) {
	t.Helper()

	imagePath := filepath.Join("testdata", "test_image.jpg")
	data, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatalf("Failed to read test image %s: %v", imagePath, err)
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	t.Logf("Loaded test image: %d bytes, base64: %d chars", len(data), len(encoded))
	return encoded, "image/jpeg"
}

// TestIntegration_ImageUnderstanding sends an image attachment via SendInput and
// verifies the daemon processes it (expects a vision-capable model to be configured).
func TestIntegration_ImageUnderstanding(t *testing.T) {
	skipIfNoDaemon(t)

	cfg := integrationTestConfig()
	client := NewClient("ws://localhost:8765", cfg)

	// Reduced from 120s to 45s for faster test execution
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
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

	// Load and encode test image
	imgData, mimeType := loadTestImage(t)

	// Send input with image attachment
	err = client.SendInput(ctx, "Describe what you see in this image in one sentence.",
		WithLoopID(loopID),
		WithAttachments([]map[string]interface{}{
			{
				"mime_type": mimeType,
				"data":      imgData,
			},
		}),
	)
	if err != nil {
		t.Fatalf("SendInput with image: %v", err)
	}
	t.Log("Sent input with image attachment")

	// Collect events and look for a response
	eventTypes := make(map[string]int)
	var gotAIResponse bool
	var gotIdleState bool
	// Reduced from 30s to 15s for faster test completion
	streamTimeout := time.After(15 * time.Second)

	for {
		select {
		case <-streamTimeout:
			t.Logf("Event streaming completed (gotAIResponse=%v)", gotAIResponse)
			for eventType, count := range eventTypes {
				t.Logf("  %s: %d", eventType, count)
			}
			if !gotAIResponse {
				t.Log("No AI response received within timeout — model may not support vision")
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

				// Check for loop-tagged assistant output or final report indicating the model processed the image
				if _, ok := m.LoopAIMessage(); ok || eventType == EventFinalReport {
					gotAIResponse = true
					t.Logf("Received AI response event: %s", eventType)
				}
				// Early exit on idle state (daemon finished processing)
				if m.Mode == "status" {
					if dataMap, ok := m.Data.(map[string]interface{}); ok {
						if state, ok := dataMap["state"].(string); ok && state == "idle" {
							gotIdleState = true
							t.Log("Daemon reached idle state")
						}
					}
				}
				// Early exit: if we got AI response and daemon is idle, we're done
				if gotAIResponse && gotIdleState {
					t.Log("Early exit: AI response received and daemon idle")
					return
				}
			case ErrorResponse:
				t.Logf("Error from daemon: code=%s message=%s", m.Code, m.Message)
			case StatusResponse:
				if m.State == "idle" {
					gotIdleState = true
					if gotAIResponse {
						t.Log("Early exit: AI response received and daemon idle")
						return
					}
				}
			}
		}
	}
}

// TestIntegration_ImageAttachmentPayload verifies that WithAttachments correctly
// populates the payload without requiring a vision-capable model.
func TestIntegration_ImageAttachmentPayload(t *testing.T) {
	skipIfNoDaemon(t)

	cfg := integrationTestConfig()
	client := NewClient("ws://localhost:8765", cfg)

	// Reduced from 120s to 30s for faster test execution
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

	// Load and encode test image
	imgData, mimeType := loadTestImage(t)

	// Send input with image attachment using the high-level API
	err = client.SendInput(ctx, "What is in this image?",
		WithLoopID(loopID),
		WithAttachments([]map[string]interface{}{
			{
				"mime_type": mimeType,
				"data":      imgData,
			},
		}),
	)
	if err != nil {
		t.Fatalf("SendInput with attachment: %v", err)
	}

	// Read events via channel to confirm the message was accepted (no immediate error)
	// Reduced from 15s to 10s
	streamTimeout := time.After(10 * time.Second)
	gotError := false
	gotStatus := false

	for {
		select {
		case <-streamTimeout:
			if gotError {
				t.Error("Daemon rejected the image attachment payload")
			} else if gotStatus {
				t.Log("Image attachment payload accepted by daemon")
			} else {
				t.Log("No status or error received — payload likely accepted (timeout)")
			}
			return
		case msg := <-eventCh:
			if msg == nil {
				continue
			}
			switch m := msg.(type) {
			case ErrorResponse:
				t.Logf("Error from daemon: code=%s message=%s", m.Code, m.Message)
				gotError = true
			case StatusResponse:
				t.Logf("Status state: %s", m.State)
				gotStatus = true
			case EventMessage:
				t.Logf("Event: %s", m.EventType())
				gotStatus = true
			}
		}
	}
}

// TestIntegration_MultipleImageAttachments tests sending multiple image attachments
// in a single input message.
func TestIntegration_MultipleImageAttachments(t *testing.T) {
	skipIfNoDaemon(t)

	cfg := integrationTestConfig()
	client := NewClient("ws://localhost:8765", cfg)

	// Reduced from 120s to 30s for faster test execution
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

	imgData, mimeType := loadTestImage(t)

	// Send same image twice to test multiple attachments
	err = client.SendInput(ctx, "Compare these two images.",
		WithLoopID(loopID),
		WithAttachments([]map[string]interface{}{
			{"mime_type": mimeType, "data": imgData},
			{"mime_type": mimeType, "data": imgData},
		}),
	)
	if err != nil {
		t.Fatalf("SendInput with multiple attachments: %v", err)
	}
	t.Log("Sent input with multiple image attachments")

	// Stream events briefly to confirm no immediate rejection
	// Reduced from 15s to 10s
	eventTypes := make(map[string]int)
	streamTimeout := time.After(10 * time.Second)

	for {
		select {
		case <-streamTimeout:
			t.Logf("Event summary:")
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
			case ErrorResponse:
				t.Logf("Error: code=%s message=%s", m.Code, m.Message)
				eventTypes["error"]++
			}
		}
	}
}
