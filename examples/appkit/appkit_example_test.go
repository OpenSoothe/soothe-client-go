package appkit_test

import (
	"context"
	"fmt"
	"time"

	"github.com/mirasoth/soothe-client-go/appkit"
	soothe "github.com/mirasoth/soothe-client-go"
)

// Example_sseBroadcaster demonstrates the SSE fan-out broadcaster for
// delivering streaming events to subscribers keyed by session id.
func Example_sseBroadcaster() {
	b := appkit.NewSSEBroadcaster()

	// Two subscribers for the same session.
	ch1, _ := b.Subscribe("session-1")
	ch2, _ := b.Subscribe("session-1")

	// Broadcast an event — both subscribers receive it.
	b.Broadcast("session-1", appkit.SSEEvent{Type: "delta", Data: "hello"})
	fmt.Printf("ch1: %v\n", <-ch1)
	fmt.Printf("ch2: %v\n", <-ch2)

	// Unsubscribe one subscriber.
	b.Unsubscribe("session-1", ch1)

	// Broadcast again — only ch2 receives it.
	b.Broadcast("session-1", appkit.SSEEvent{Type: "complete", Data: "done"})
	fmt.Printf("ch2: %v\n", <-ch2)

	// Close removes all subscribers for a session.
	b.Close("session-1")
	fmt.Println("closed")
	// Output:
	// ch1: {delta hello}
	// ch2: {delta hello}
	// ch2: {complete done}
	// closed
}

// Example_queryGate demonstrates single-flight query enforcement: only one
// query per session at a time, with cancel-before-context ordering.
func Example_queryGate() {
	gate := appkit.NewQueryGate()

	ctx, cancel := context.WithCancel(context.Background())
	sendCancel := func(c context.Context) error {
		fmt.Println("daemon cancel sent")
		return nil
	}

	// Acquire the gate for session-1.
	if err := gate.Acquire("session-1", cancel, sendCancel); err != nil {
		fmt.Printf("Acquire error: %v\n", err)
	}
	fmt.Printf("Active: %v\n", gate.IsActive("session-1"))

	// A second acquire for the same session fails with ErrQueryBusy.
	err := gate.Acquire("session-1", cancel, sendCancel)
	fmt.Printf("Second acquire: %v\n", err)

	// Cancel sends the daemon cancel first, then cancels the local context.
	_ = gate.Cancel("session-1")
	fmt.Printf("Active after cancel: %v\n", gate.IsActive("session-1"))
	_ = ctx
	// Output:
	// Active: true
	// Second acquire: appkit: query already in progress for session
	// daemon cancel sent
	// Active after cancel: false
}

// Example_eventClassifier demonstrates classifying daemon stream events into
// deliverable/streaming/terminal outcomes using a configurable phase set.
func Example_eventClassifier() {
	cl := appkit.NewEventClassifier(appkit.ClassifierConfig{
		DeliverablePhases: soothe.DefaultDeliverablePhases(),
	})

	// A loop-tagged message with a deliverable phase.
	em := soothe.EventMessage{}
	em.BaseMessage = soothe.BaseMessage{Type: "event"}
	em.Mode = "messages"
	em.Data = []interface{}{
		map[string]interface{}{
			"type":    "ai",
			"content": "Here is the final answer to your question.",
			"phase":   "text_completion",
		},
	}

	// Classify the event.
	result := cl.Classify(em, "")
	fmt.Printf("Terminal: %d\n", result.Terminal)
	fmt.Printf("Content: %s\n", result.Content)
	fmt.Printf("CompletionEvent: %s\n", result.CompletionEvent)

	// Check if the completion event is deliverable.
	isDeliverable := cl.IsDeliverableCompletionEvent(result.CompletionEvent)
	fmt.Printf("IsDeliverable: %v\n", isDeliverable)

	// Resolve the final user-facing content.
	final, ok := cl.ResolveDeliverableFinalContent(result, "")
	fmt.Printf("Final: %s ok=%v\n", final, ok)

	// Check substantive reply length.
	fmt.Printf("Substantive: %v\n", cl.IsSubstantiveAssistantReply("hi"))       // too short
	fmt.Printf("Substantive: %v\n", cl.IsSubstantiveAssistantReply("A full reply here.")) // ok
	// Output:
	// Terminal: 1
	// Content: Here is the final answer to your question.
	// CompletionEvent: soothe.protocol.message.text_completion
	// IsDeliverable: true
	// Final: Here is the final answer to your question. ok=true
	// Substantive: false
	// Substantive: true
}

// Example_inputMessageForLoop builds a loop_input payload map with optional
// attachments and daemon hints, without needing a live connection.
func Example_inputMessageForLoop() {
	// Simple text input.
	msg := appkit.InputMessageForLoop("What is 2+2?", "loop-123", nil, nil)
	fmt.Printf("type=%s content=%s loop_id=%s\n",
		msg["type"], msg["content"], msg["loop_id"])

	// With intent hint and preferred subagent.
	msg2 := appkit.InputMessageForLoop("Analyze this image", "loop-456",
		[]map[string]interface{}{
			{"mime_type": "image/png", "data": "base64data"},
		},
		&appkit.InputOpts{
			IntentHint:        soothe.IntentHintImageToText,
			PreferredSubagent: "explore",
		})
	fmt.Printf("intent_hint=%s subagent=%s attachments=%v\n",
		msg2["intent_hint"], msg2["preferred_subagent"], msg2["attachments"] != nil)

	// With response schema for structured output.
	strict := true
	msg3 := appkit.InputMessageForLoop("Extract data", "loop-789", nil,
		&appkit.InputOpts{
			IntentHint:           soothe.IntentHintTextCompletion,
			ResponseSchema:       map[string]interface{}{"type": "object"},
			ResponseSchemaName:   "my_schema",
			ResponseSchemaStrict: &strict,
		})
	fmt.Printf("schema_name=%s strict=%v\n",
		msg3["response_schema_name"], msg3["response_schema_strict"])
	// Output:
	// type=loop_input content=What is 2+2? loop_id=loop-123
	// intent_hint=image_to_text subagent=explore attachments=true
	// schema_name=my_schema strict=true
}

// Example_poolConfig demonstrates the ConnectionPool configuration types and
// default values.
func Example_poolConfig() {
	cfg := appkit.DefaultPoolConfig()
	fmt.Printf("PoolSize: %d\n", cfg.PoolSize)
	fmt.Printf("QueryTimeout: %v\n", cfg.QueryTimeout)
	fmt.Printf("ConnectionTimeout: %v\n", cfg.ConnectionTimeout)
	fmt.Printf("MaxIdleTime: %v\n", cfg.MaxIdleTime)
	fmt.Printf("HealthCheckInterval: %v\n", cfg.HealthCheckInterval)
	// Output:
	// PoolSize: 1000
	// QueryTimeout: 30m0s
	// ConnectionTimeout: 30s
	// MaxIdleTime: 10m0s
	// HealthCheckInterval: 30s
}

// Example_connectionPoolConstruction shows how to construct a ConnectionPool
// with a custom SessionStore and configuration. A running daemon is required
// for Acquire; this example shows only the construction and Stats.
func Example_connectionPoolConstruction() {
	// In-memory session store for demonstration.
	store := &memorySessionStore{}

	pool := appkit.NewConnectionPool(
		"ws://localhost:8765",
		store,
		&appkit.PoolConfig{
			PoolSize:          2,
			QueryTimeout:      5 * time.Minute,
			ConnectionTimeout: 10 * time.Second,
		},
		soothe.DefaultConfig(),
		nil, // nil factory → DefaultClientFactory
	)

	// Override the bootstrap function for a custom workspace setup.
	pool = pool.WithBootstrap(func(
		ctx context.Context,
		client appkit.ManagedClient,
		workspaceID, userID string,
		cfg *soothe.Config,
	) (string, error) {
		return "", fmt.Errorf("demo: no live daemon")
	})

	active, idle := pool.Stats()
	fmt.Printf("Active: %d Idle: %d\n", active, idle)

	pool.Stop()
	fmt.Println("stopped")
	// Output:
	// Active: 0 Idle: 2
	// stopped
}

// Example_turnRunnerConstruction shows how to wire up a TurnRunner with all
// its dependencies. Execute requires a live daemon; this example shows the
// builder pattern for hooks.
func Example_turnRunnerConstruction() {
	store := &memorySessionStore{}
	pool := appkit.NewConnectionPool(
		"ws://localhost:8765", store,
		&appkit.PoolConfig{PoolSize: 1},
		soothe.DefaultConfig(), nil,
	)
	defer pool.Stop()

	gate := appkit.NewQueryGate()
	cl := appkit.NewEventClassifier(appkit.ClassifierConfig{
		DeliverablePhases: soothe.DefaultDeliverablePhases(),
	})
	broadcaster := appkit.NewSSEBroadcaster()

	runner := appkit.NewTurnRunner(
		pool, gate, cl, store, broadcaster,
		appkit.TurnConfig{QueryTimeout: 10 * time.Minute},
	)

	// Register completion and error hooks.
	runner = runner.WithOnComplete(
		func(sessionID, loopID, content, completionEvent string, elapsedMs int64) {
			fmt.Printf("completed: %s (%dms)\n", sessionID, elapsedMs)
		},
	).WithOnError(
		func(sessionID, loopID string, err error) {
			fmt.Printf("error: %s: %v\n", sessionID, err)
		},
	)

	// Override the input builder to inject custom fields.
	runner = runner.WithInputBuilder(
		func(text, loopID string, attachments []map[string]interface{}, opts *appkit.InputOpts) map[string]interface{} {
			msg := appkit.InputMessageForLoop(text, loopID, attachments, opts)
			msg["custom_field"] = "injected"
			return msg
		},
	)

	fmt.Println("TurnRunner ready")
	_ = runner // would call runner.Execute(ctx, ...) with a live daemon
	// Output:
	// TurnRunner ready
}

// Example_chatEventTerminal shows the terminal classification constants.
func Example_chatEventTerminal() {
	fmt.Println(int(appkit.ChatEventContinue))              // 0
	fmt.Println(int(appkit.ChatEventDeliverableComplete))   // 1
	fmt.Println(int(appkit.ChatEventFailedComplete))        // 2
	// Output:
	// 0
	// 1
	// 2
}

// Example_sessionStoreInterface shows the SessionStore interface methods and
// the SessionEntry/SessionMessage types.
func Example_sessionStoreInterface() {
	// SessionEntry is the persisted session↔loop mapping.
	entry := appkit.SessionEntry{
		WorkspaceID: "/tmp/project",
		SessionID:   "sess-123",
		LoopID:      "loop-abc",
		SessionType: "primary",
		IsActive:    true,
		ResetCount:  0,
	}
	fmt.Printf("Session=%s Loop=%s Active=%v\n",
		entry.SessionID, entry.LoopID, entry.IsActive)

	// SessionMessage is a persisted message row.
	msg := appkit.SessionMessage{
		ID:       "msg-1",
		Role:     "assistant",
		Content:  "Here is the answer.",
		Metadata: map[string]interface{}{"elapsed_ms": 1234},
	}
	fmt.Printf("Msg role=%s content=%s\n", msg.Role, msg.Content)
	// Output:
	// Session=sess-123 Loop=loop-abc Active=true
	// Msg role=assistant content=Here is the answer.
}

// Example_managedClientInterface shows the ManagedClient interface and factory
// / bootstrap function types.
func Example_managedClientInterface() {
	// DefaultClientFactory builds a *soothe.Client.
	factory := appkit.DefaultClientFactory()
	client := factory("ws://localhost:8765", soothe.DefaultConfig())
	fmt.Printf("Connected: %v\n", client.IsConnected())

	// DefaultBootstrapFunc calls soothe.BootstrapLoopSession on the
	// underlying *soothe.Client.
	_ = appkit.DefaultBootstrapFunc()
	fmt.Println("factory and bootstrap ready")
	// Output:
	// Connected: false
	// factory and bootstrap ready
}

// --- in-memory SessionStore for demonstration ---

type memorySessionStore struct {
	sessions map[string]*appkit.SessionEntry
}

func (s *memorySessionStore) GetSession(_ context.Context, sessionID string) (*appkit.SessionEntry, error) {
	if s.sessions == nil {
		s.sessions = make(map[string]*appkit.SessionEntry)
	}
	if e, ok := s.sessions[sessionID]; ok {
		return e, nil
	}
	return nil, nil
}

func (s *memorySessionStore) CreateSession(_ context.Context, workspaceID, sessionID, loopID, sessionType string) error {
	if s.sessions == nil {
		s.sessions = make(map[string]*appkit.SessionEntry)
	}
	s.sessions[sessionID] = &appkit.SessionEntry{
		WorkspaceID: workspaceID,
		SessionID:   sessionID,
		LoopID:      loopID,
		SessionType: sessionType,
		IsActive:    true,
	}
	return nil
}

func (s *memorySessionStore) UpdateLastUsed(_ context.Context, _ string) error {
	return nil
}

func (s *memorySessionStore) IncrementResetCount(_ context.Context, sessionID string) error {
	if e, ok := s.sessions[sessionID]; ok {
		e.ResetCount++
	}
	return nil
}

func (s *memorySessionStore) GetLoopIDForSession(_ context.Context, sessionID string) (string, bool, error) {
	if e, ok := s.sessions[sessionID]; ok && e.LoopID != "" {
		return e.LoopID, true, nil
	}
	return "", false, nil
}

func (s *memorySessionStore) AppendMessage(_ context.Context, _ string, _ appkit.SessionMessage) error {
	return nil
}
