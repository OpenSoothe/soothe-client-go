package soothe

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// messagesModeAssistantContent extracts plain assistant text from a daemon stream
// event with mode "messages" and data shaped as [messageDict, metadata]. This
// covers direct-model turns (intent_hint direct_llm / image_to_text) that emit a
// single AIMessage without loop-tagged phase metadata.
func messagesModeAssistantContent(m EventMessage) (string, bool) {
	if m.Mode != "messages" {
		return "", false
	}
	items, ok := m.Data.([]interface{})
	if !ok || len(items) == 0 {
		return "", false
	}
	msgMap, ok := items[0].(map[string]interface{})
	if !ok {
		return "", false
	}
	ctype, _ := msgMap["type"].(string)
	if ctype != "" {
		switch ctype {
		case "ai", "AIMessage", "assistant":
			// ok
		default:
			return "", false
		}
	}
	switch c := msgMap["content"].(type) {
	case string:
		s := strings.TrimSpace(c)
		if s != "" {
			return s, true
		}
	}
	return "", false
}

func TestIntegration_IntentHintDirectLLM(t *testing.T) {
	skipIfNoDaemon(t)

	cfg := integrationTestConfig()
	client := NewClient(cfg.DaemonURL, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	wsDir := t.TempDir()
	loopID, err := BootstrapLoopSession(ctx, client, "", wsDir, cfg)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	t.Logf("loop_id=%s", loopID)

	ch, err := client.ReceiveMessages(ctx)
	if err != nil {
		t.Fatalf("ReceiveMessages: %v", err)
	}

	prompt := "Reply with exactly one word: OK."
	if err := client.SendInput(ctx, prompt, WithLoopID(loopID), WithIntentHint("direct_llm")); err != nil {
		t.Fatalf("SendInput direct_llm: %v", err)
	}

	deadline := time.After(90 * time.Second)
	var sawRunning, sawIdle bool
	var assistant string
	var errResp *ErrorResponse

	for {
		select {
		case <-deadline:
			if errResp != nil {
				t.Fatalf("daemon error: %s — %s", errResp.Code, errResp.Message)
			}
			if assistant == "" {
				t.Fatalf("timeout: expected mode=messages assistant content (got running=%v idle=%v)", sawRunning, sawIdle)
			}
			t.Logf("direct_llm assistant: %q", assistant)
			return
		case msg, ok := <-ch:
			if !ok {
				if assistant != "" {
					t.Logf("direct_llm assistant: %q", assistant)
					return
				}
				t.Fatal("channel closed before assistant message")
			}
			if msg == nil {
				continue
			}
			switch m := msg.(type) {
			case EventMessage:
				if txt, ok := messagesModeAssistantContent(m); ok {
					assistant = txt
					t.Logf("messages assistant: %q", assistant)
					return
				}
				if m.Mode == "custom" {
					if et := m.EventType(); et != "" && !strings.HasPrefix(et, "soothe.internal.") {
						t.Logf("custom event: %s", et)
					}
				}
			case StatusResponse:
				switch m.State {
				case "running":
					sawRunning = true
				case "idle":
					sawIdle = true
					if assistant != "" {
						t.Logf("direct_llm assistant: %q", assistant)
						return
					}
				}
			case ErrorResponse:
				e := m
				errResp = &e
				t.Logf("error frame: %s %s", m.Code, m.Message)
			}
		}
	}
}

func TestIntegration_IntentHintImageToText(t *testing.T) {
	skipIfNoDaemon(t)

	cfg := integrationTestConfig()
	client := NewClient(cfg.DaemonURL, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	wsDir := t.TempDir()
	loopID, err := BootstrapLoopSession(ctx, client, "", wsDir, cfg)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	t.Logf("loop_id=%s", loopID)

	ch, err := client.ReceiveMessages(ctx)
	if err != nil {
		t.Fatalf("ReceiveMessages: %v", err)
	}

	imgData, mimeType := loadTestImage(t)

	err = client.SendInput(ctx, "In one short phrase, what is shown?",
		WithLoopID(loopID),
		WithIntentHint("image_to_text"),
		WithAttachments([]map[string]interface{}{
			{"mime_type": mimeType, "data": imgData},
		}),
	)
	if err != nil {
		t.Fatalf("SendInput image_to_text: %v", err)
	}

	deadline := time.After(90 * time.Second)
	var assistant string

	for {
		select {
		case <-deadline:
			if assistant == "" {
				t.Fatal("timeout: expected mode=messages assistant content for image_to_text")
			}
			t.Logf("image_to_text: %q", assistant)
			return
		case msg, ok := <-ch:
			if !ok {
				if assistant != "" {
					t.Logf("image_to_text: %q", assistant)
					return
				}
				t.Fatal("channel closed before assistant message")
			}
			if msg == nil {
				continue
			}
			switch m := msg.(type) {
			case EventMessage:
				if txt, ok := messagesModeAssistantContent(m); ok {
					assistant = txt
					t.Logf("messages assistant: %q", assistant)
					return
				}
			case ErrorResponse:
				t.Fatalf("daemon error: %s — %s", m.Code, m.Message)
			case StatusResponse:
				if m.State == "idle" && assistant != "" {
					t.Logf("image_to_text: %q", assistant)
					return
				}
			}
		}
	}
}

func TestIntegration_IntentHintImageToText_RejectsWithoutAttachments(t *testing.T) {
	skipIfNoDaemon(t)

	cfg := integrationTestConfig()
	client := NewClient(cfg.DaemonURL, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	wsDir := t.TempDir()
	loopID, err := BootstrapLoopSession(ctx, client, "", wsDir, cfg)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	if err := client.SendInput(ctx, "text without image",
		WithLoopID(loopID),
		WithIntentHint("image_to_text"),
	); err != nil {
		t.Fatalf("SendInput: %v", err)
	}

	// Read with ReadEvent only. Starting ReceiveMessages spawns a second reader on the
	// same WebSocket; competing ReadMessage calls are unsafe and can drop frames.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if client.conn != nil {
			_ = client.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		}
		ev, err := client.ReadEvent()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if ev == nil {
			continue
		}
		typ, _ := ev["type"].(string)
		switch typ {
		case "error":
			code, _ := ev["code"].(string)
			msgStr, _ := ev["message"].(string)
			if code != "INVALID_REQUEST" {
				t.Fatalf("unexpected error code %q: %s", code, msgStr)
			}
			if !strings.Contains(strings.ToLower(msgStr), "attachment") {
				t.Fatalf("unexpected error message: %s", msgStr)
			}
			t.Logf("got expected rejection: %s", msgStr)
			return
		case "loop_input_response":
			// Older daemons (before intent_hint image_to_text validation) still enqueue and
			// ack; skip so integration suites pass against mixed daemon versions.
			if ok, _ := ev["success"].(bool); ok {
				t.Skip(
					"daemon accepted image_to_text without attachments (loop_input_response); " +
						"upgrade soothe-daemon with intent_hint image_to_text validation",
				)
			}
			continue
		default:
			continue
		}
	}
	t.Fatal("timeout: expected INVALID_REQUEST error for image_to_text without attachments")
}

var integrationWordReplySchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"word": map[string]interface{}{"type": "string"},
	},
	"required":             []interface{}{"word"},
	"additionalProperties": false,
}

func TestIntegration_IntentHintDirectLLMStructured(t *testing.T) {
	skipIfNoDaemon(t)

	cfg := integrationTestConfig()
	client := NewClient(cfg.DaemonURL, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	wsDir := t.TempDir()
	loopID, err := BootstrapLoopSession(ctx, client, "", wsDir, cfg)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	ch, err := client.ReceiveMessages(ctx)
	if err != nil {
		t.Fatalf("ReceiveMessages: %v", err)
	}

	strict := true
	prompt := `Return JSON only. Set "word" to exactly "GOJSON".`
	if err := client.SendInput(ctx, prompt,
		WithLoopID(loopID),
		WithIntentHint("direct_llm"),
		WithResponseSchema(integrationWordReplySchema),
		WithResponseSchemaName("WordReply"),
		WithResponseSchemaStrict(strict),
	); err != nil {
		t.Fatalf("SendInput structured direct_llm: %v", err)
	}

	deadline := time.After(90 * time.Second)
	var assistant string
	var errResp *ErrorResponse

	for {
		select {
		case <-deadline:
			if errResp != nil {
				t.Fatalf("daemon error: %s — %s", errResp.Code, errResp.Message)
			}
			if assistant == "" {
				t.Fatal("timeout: expected structured JSON assistant content")
			}
			var parsed struct {
				Word string `json:"word"`
			}
			if err := json.Unmarshal([]byte(assistant), &parsed); err != nil {
				t.Fatalf("assistant is not valid JSON: %q err=%v", assistant, err)
			}
			if !strings.Contains(strings.ToUpper(parsed.Word), "GOJSON") {
				t.Fatalf("word field: got %q", parsed.Word)
			}
			t.Logf("structured direct_llm: %q", assistant)
			return
		case msg, ok := <-ch:
			if !ok {
				if assistant != "" {
					return
				}
				t.Fatal("channel closed before assistant message")
			}
			if msg == nil {
				continue
			}
			switch m := msg.(type) {
			case EventMessage:
				if txt, ok := messagesModeAssistantContent(m); ok {
					assistant = txt
				}
			case ErrorResponse:
				e := m
				errResp = &e
			}
		}
	}
}

func TestIntegration_IntentHintDirectLLMStructured_RejectsWithoutDirectLLM(t *testing.T) {
	skipIfNoDaemon(t)

	cfg := integrationTestConfig()
	client := NewClient(cfg.DaemonURL, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	wsDir := t.TempDir()
	loopID, err := BootstrapLoopSession(ctx, client, "", wsDir, cfg)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	if err := client.SendInput(ctx, "hello",
		WithLoopID(loopID),
		WithIntentHint("quiz"),
		WithResponseSchema(integrationWordReplySchema),
	); err != nil {
		t.Fatalf("SendInput: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if client.conn != nil {
			_ = client.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		}
		ev, err := client.ReadEvent()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if ev == nil {
			continue
		}
		typ, _ := ev["type"].(string)
		switch typ {
		case "error":
			code, _ := ev["code"].(string)
			msgStr, _ := ev["message"].(string)
			if code != "INVALID_REQUEST" {
				t.Fatalf("unexpected error code %q: %s", code, msgStr)
			}
			if !strings.Contains(strings.ToLower(msgStr), "direct_llm") {
				t.Fatalf("unexpected error message: %s", msgStr)
			}
			t.Logf("got expected rejection: %s", msgStr)
			return
		case "loop_input_response":
			if ok, _ := ev["success"].(bool); ok {
				t.Skip(
					"daemon accepted response_schema without direct_llm; upgrade soothe-daemon",
				)
			}
		}
	}
	t.Fatal("timeout: expected INVALID_REQUEST for response_schema with intent_hint=quiz")
}
