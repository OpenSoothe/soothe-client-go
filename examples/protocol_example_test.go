package soothe_test

import (
	"encoding/json"
	"fmt"

	soothe "github.com/mirasoth/soothe-client-go"
)

// Example_envelopeConstructors demonstrates the protocol-1 wire envelope
// constructors used to build raw messages for SendMessage.
func Example_envelopeConstructors() {
	// Request envelope with auto-generated id.
	req := soothe.NewRequestEnvelope("daemon_status", nil)
	fmt.Printf("Request: proto=%s type=%s method=%s id_len=%d\n",
		req.Proto, req.Type, req.Method, len(req.ID))

	// Request envelope with explicit id for correlation.
	reqWithID := soothe.NewRequestEnvelopeWithID("loop_get",
		map[string]interface{}{"loop_id": "abc-123"}, "my-req-id")
	fmt.Printf("Request with id: id=%s method=%s\n", reqWithID.ID, reqWithID.Method)

	// Notification envelope (fire-and-forget, no id).
	notif := soothe.NewNotificationEnvelope("loop_input",
		map[string]interface{}{"loop_id": "abc-123", "content": "hello"})
	fmt.Printf("Notification: type=%s method=%s id_empty=%v\n",
		notif.Type, notif.Method, notif.ID == "")

	// Subscribe envelope.
	sub := soothe.NewSubscribeEnvelope("loop_events",
		map[string]interface{}{"loop_id": "abc-123"})
	fmt.Printf("Subscribe: type=%s method=%s\n", sub.Type, sub.Method)

	// Unsubscribe envelope by subscription id.
	unsub := soothe.NewUnsubscribeEnvelope("sub-id-123")
	fmt.Printf("Unsubscribe: type=%s id=%s\n", unsub.Type, unsub.ID)

	// Connection init handshake envelope.
	init := soothe.NewConnectionInitEnvelope()
	fmt.Printf("ConnectionInit: type=%s proto=%s\n", init.Type, init.Proto)

	// Heartbeat envelopes.
	ping := soothe.NewPingEnvelope()
	pong := soothe.NewPongEnvelope()
	fmt.Printf("Ping: type=%s Pong: type=%s\n", ping.Type, pong.Type)

	// Disconnect notification envelope.
	disc := soothe.NewDisconnectEnvelope()
	fmt.Printf("Disconnect: type=%s\n", disc.Type)

	// NewRequestID generates a UUID for correlation.
	id := soothe.NewRequestID()
	fmt.Printf("RequestID length: %d\n", len(id))
	// Output:
	// Request: proto=1 type=request method=daemon_status id_len=36
	// Request with id: id=my-req-id method=loop_get
	// Notification: type=notification method=loop_input id_empty=true
	// Subscribe: type=subscribe method=loop_events
	// Unsubscribe: type=unsubscribe id=sub-id-123
	// ConnectionInit: type=connection_init proto=1
	// Ping: type=ping Pong: type=pong
	// Disconnect: type=disconnect
	// RequestID length: 36
}

// Example_decodeMessage decodes raw JSON wire bytes into typed Go structs.
// Protocol-1 envelopes are decoded into the unified Envelope struct; legacy
// flat-form frames are decoded into their typed structs.
func Example_decodeMessage() {
	// Decode a protocol-1 request envelope.
	raw := []byte(`{"proto":"1","type":"request","method":"daemon_status","id":"req-1"}`)
	msg, err := soothe.DecodeMessage(raw)
	if err != nil {
		fmt.Printf("Decode error: %v\n", err)
		return
	}
	env, ok := msg.(soothe.Envelope)
	fmt.Printf("Decoded: type=%s method=%s id=%s isEnvelope=%v\n",
		env.Type, env.Method, env.ID, ok)

	// Decode a notification envelope.
	notifRaw := []byte(`{"proto":"1","type":"notification","method":"loop_input","params":{"content":"hi"}}`)
	msg2, _ := soothe.DecodeMessage(notifRaw)
	env2, _ := msg2.(soothe.Envelope)
	fmt.Printf("Notif: type=%s method=%s\n", env2.Type, env2.Method)
	// Output:
	// Decoded: type=request method=daemon_status id=req-1 isEnvelope=true
	// Notif: type=notification method=loop_input
}

// Example_splitSootheWirePayload handles NDJSON: the daemon may send
// newline-delimited JSON in a single WebSocket text frame.
func Example_splitSootheWirePayload() {
	ndjson := []byte(`{"type":"ping"}` + "\n" + `{"type":"pong"}`)
	parts := soothe.SplitSootheWirePayload(ndjson)
	fmt.Printf("Parts: %d\n", len(parts))
	for i, p := range parts {
		fmt.Printf("  [%d] %s\n", i, string(p))
	}
	// Output:
	// Parts: 2
	//   [0] {"type":"ping"}
	//   [1] {"type":"pong"}
}

// Example_parseNamespace splits a 4-segment event namespace into components.
// Returns ok=false for internal.* namespaces (server-only).
func Example_parseNamespace() {
	ns := "soothe.cognition.plan.created"
	domain, component, action, ok := soothe.ParseNamespace(ns)
	fmt.Printf("Domain=%s Component=%s Action=%s ok=%v\n", domain, component, action, ok)

	// Internal namespaces are rejected (server-only, not for clients).
	_, _, _, okInternal := soothe.ParseNamespace("soothe.internal.daemon.heartbeat")
	fmt.Printf("Internal ok=%v\n", okInternal)
	// Output:
	// Domain=cognition Component=plan Action=created ok=true
	// Internal ok=false
}

// Example_eventConstants shows commonly used event namespace constants for
// dispatching streaming events from the daemon.
func Example_eventConstants() {
	// Plan events.
	fmt.Println(soothe.EventPlanCreated)
	fmt.Println(soothe.EventPlanBatchStarted)

	// Goal events.
	fmt.Println(soothe.EventGoalCompleted)
	fmt.Println(soothe.EventGoalFailed)

	// Tool events.
	fmt.Println(soothe.EventToolStarted)
	fmt.Println(soothe.EventToolCompleted)

	// StrangeLoop events.
	fmt.Println(soothe.EventStrangeLoopStarted)
	fmt.Println(soothe.EventStrangeLoopCompleted)

	// Autopilot events.
	fmt.Println(soothe.EventAutopilotStatusChanged)
	fmt.Println(soothe.EventAutopilotGoalCompleted)

	// Output events.
	fmt.Println(soothe.EventFinalReport)
	// Output:
	// soothe.cognition.plan.created
	// soothe.cognition.plan.batch.started
	// soothe.cognition.goal.completed
	// soothe.cognition.goal.failed
	// soothe.tool.execution.started
	// soothe.tool.execution.completed
	// soothe.cognition.strange_loop.started
	// soothe.cognition.strange_loop.completed
	// soothe.system.autopilot.status.changed
	// soothe.system.autopilot.goal.completed
	// soothe.output.autonomous.final_report.reported
}

// Example_extractSootheLoopID extracts a loop id from various daemon message
// shapes (protocol-1 envelopes, EventMessage projections, generic maps).
func Example_extractSootheLoopID() {
	// From a next envelope payload. DecodeMessage may project next frames
	// into EventMessage — ExtractSootheLoopID handles all decoded types.
	nextData := []byte(`{"proto":"1","type":"next","id":"sub-1","payload":{"data":{"loop_id":"loop-abc"}}}`)
	msg, _ := soothe.DecodeMessage(nextData)
	loopID, ok := soothe.ExtractSootheLoopID(msg)
	fmt.Printf("LoopID=%s ok=%v\n", loopID, ok)

	// From a generic map.
	m := map[string]interface{}{"type": "next", "payload": map[string]interface{}{
		"data": map[string]interface{}{"loop_id": "loop-xyz"},
	}}
	loopID2, ok2 := soothe.ExtractSootheLoopID(m)
	fmt.Printf("LoopID=%s ok=%v\n", loopID2, ok2)
	// Output:
	// LoopID=loop-abc ok=true
	// LoopID=loop-xyz ok=true
}

// Example_expandWireMessages flattens event_batch envelopes into individual
// decoded messages.
func Example_expandWireMessages() {
	batchJSON := `{"type":"event_batch","events":[` +
		`{"proto":"1","type":"ping"},` +
		`{"proto":"1","type":"pong"}` +
		`]}`
	var batch map[string]interface{}
	_ = json.Unmarshal([]byte(batchJSON), &batch)
	expanded := soothe.ExpandWireMessages(batch)
	fmt.Printf("Expanded: %d messages\n", len(expanded))
	for i, m := range expanded {
		if env, ok := m.(soothe.Envelope); ok {
			fmt.Printf("  [%d] type=%s\n", i, env.Type)
		}
	}
	// Output:
	// Expanded: 2 messages
	//   [0] type=ping
	//   [1] type=pong
}

// Example_eventMessageMethods demonstrates EventMessage helper methods for
// inspecting decoded streaming events.
func Example_eventMessageMethods() {
	// Build an EventMessage with a string namespace.
	em := soothe.EventMessage{}
	em.BaseMessage = soothe.BaseMessage{Type: "event"}
	em.Namespace = "soothe.cognition.plan.created"
	em.Data = map[string]interface{}{"type": "soothe.cognition.plan.created"}

	// EventType returns the normalized event type.
	fmt.Printf("EventType: %s\n", em.EventType())

	// NamespaceParts returns the dot-split segments.
	parts := em.NamespaceParts()
	fmt.Printf("Parts: %v\n", parts)

	// ParseNamespace works on the string form too.
	domain, component, action, ok := soothe.ParseNamespace(em.Namespace.(string))
	fmt.Printf("Parsed: %s.%s.%s ok=%v\n", domain, component, action, ok)
	// Output:
	// EventType: soothe.cognition.plan.created
	// Parts: [soothe cognition plan created]
	// Parsed: cognition.plan.created ok=true
}

// Example_loopAIMessage shows extracting loop-tagged assistant messages from
// mode="messages" events, and extracting plain text from their content.
func Example_loopAIMessage() {
	// A mode="messages" event with a loop-tagged AIMessage.
	em := soothe.EventMessage{}
	em.BaseMessage = soothe.BaseMessage{Type: "event"}
	em.Mode = "messages"
	em.Data = []interface{}{
		map[string]interface{}{
			"type":    "ai",
			"content": "Here is the answer to your question.",
			"phase":   "text_completion",
		},
	}

	// LoopAIMessage extracts the tagged assistant payload.
	aiMsg, ok := em.LoopAIMessage()
	if !ok {
		fmt.Println("Not a loop AI message")
		return
	}
	fmt.Printf("Type=%s Phase=%s\n", aiMsg.Type, aiMsg.Phase)
	fmt.Printf("Text=%s\n", aiMsg.LoopAIText())
	// Output:
	// Type=ai Phase=text_completion
	// Text=Here is the answer to your question.
}
