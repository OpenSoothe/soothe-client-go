package appkit

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	soothe "github.com/mirasoth/soothe-client-go"
)

// ChatEventTerminal classifies how a processed event should end the query loop.
type ChatEventTerminal int

const (
	// ChatEventContinue — accumulate content; the query is still running.
	ChatEventContinue ChatEventTerminal = iota
	// ChatEventDeliverableComplete — a user-visible final reply; persist it.
	ChatEventDeliverableComplete
	// ChatEventFailedComplete — the query failed; persist an error.
	ChatEventFailedComplete
)

// ChatEventResult is the structured outcome of classifying one daemon event.
type ChatEventResult struct {
	Content         string
	ThinkingStep    string // user-visible progress line (not a final reply)
	Terminal        ChatEventTerminal
	CompletionEvent string // soothe wire event type when Terminal == ChatEventDeliverableComplete
	Err             error
}

// ClassifierConfig supplies the product-specific decisions an EventClassifier
// needs. The DeliverablePhases set is the key product knob: which message
// `phase` values count as user-facing deliverables (triarch uses
// {quiz, goal_completion, direct_model}; other apps pass their own).
type ClassifierConfig struct {
	// DeliverablePhases recognizes loop-tagged message phases that may end a
	// query with user-facing text. Required.
	DeliverablePhases map[string]bool

	// MinDeliverableRunes is the minimum trimmed rune count for a reply to be
	// persisted as final (avoids finishing on stub ACKs like "..."). Defaults
	// to 8 if zero.
	MinDeliverableRunes int

	// ThinkingStepEvents, if non-nil, overrides the default allowlist of event
	// types mapped to a one-line thinking step (RFC-629 open question). If nil,
	// the library default allowlist is used.
	ThinkingStepEvents map[string]bool
}

// EventClassifier maps a stream of decoded daemon events into
// deliverable/streaming/terminal outcomes, keyed on (namespace, mode, phase)
// per RFC-614/RFC-403 (RFC-629 constraint #4). It is the app-agnostic
// successor to triarch's ProcessChatEvent, with the deliverable phase set
// promoted from hardcoded constants to configuration.
type EventClassifier struct {
	cfg ClassifierConfig
}

// NewEventClassifier constructs a classifier from config. Panics if
// DeliverablePhases is nil (it is the one required product decision).
func NewEventClassifier(cfg ClassifierConfig) *EventClassifier {
	if cfg.DeliverablePhases == nil {
		panic("appkit: ClassifierConfig.DeliverablePhases must not be nil")
	}
	if cfg.MinDeliverableRunes <= 0 {
		cfg.MinDeliverableRunes = 8
	}
	return &EventClassifier{cfg: cfg}
}

// Classify inspects one decoded event and returns its outcome. accumulated is
// the running assistant text so far, used to pick the final reply when a
// deliverable event arrives.
func (cl *EventClassifier) Classify(msg interface{}, accumulated string) ChatEventResult {
	return cl.processChatEvent(msg, accumulated)
}

// IsDeliverableCompletionEvent reports whether a persisted completion_event is
// user-facing. Uses the configured deliverable phase set; recognizes the
// protocol output namespace and final_report component as deliverable.
func (cl *EventClassifier) IsDeliverableCompletionEvent(eventType string) bool {
	if eventType == "" {
		return false
	}
	if eventType == soothe.EventFinalReport {
		return true
	}
	if strings.HasPrefix(eventType, "soothe.protocol.message.") {
		phase := strings.TrimPrefix(eventType, "soothe.protocol.message.")
		return cl.isDeliverableLoopPhase(phase)
	}
	return strings.Contains(eventType, "soothe.output") && strings.Contains(eventType, "responded")
}

func (cl *EventClassifier) isDeliverableLoopPhase(phase string) bool {
	return cl.cfg.DeliverablePhases[phase]
}

func (cl *EventClassifier) deliverableResult(content, completionEvent string) ChatEventResult {
	return ChatEventResult{
		Content:         content,
		Terminal:        ChatEventDeliverableComplete,
		CompletionEvent: completionEvent,
	}
}

func (cl *EventClassifier) continueResult(content string) ChatEventResult {
	return ChatEventResult{Content: content, Terminal: ChatEventContinue}
}

func (cl *EventClassifier) failedResult(err error) ChatEventResult {
	return ChatEventResult{Terminal: ChatEventFailedComplete, Err: err}
}

// IsSubstantiveAssistantReply reports whether trimmed assistant text is long
// enough to persist as a final reply.
func (cl *EventClassifier) IsSubstantiveAssistantReply(content string) bool {
	return len([]rune(strings.TrimSpace(content))) >= cl.cfg.MinDeliverableRunes
}

// ResolveDeliverableFinalContent picks the user-visible reply for a completed
// query. Only a deliverable terminal result with a recognized completion event
// yields a final reply.
func (cl *EventClassifier) ResolveDeliverableFinalContent(eventResult ChatEventResult, accumulated string) (final string, ok bool) {
	if eventResult.Terminal != ChatEventDeliverableComplete {
		return "", false
	}
	if !cl.IsDeliverableCompletionEvent(eventResult.CompletionEvent) {
		return "", false
	}
	if final = strings.TrimSpace(eventResult.Content); final != "" {
		return final, true
	}
	return "", false
}

func isStreamingMessageType(msgType string) bool {
	switch msgType {
	case "AIMessageChunk", "ai_chunk", "message_chunk":
		return true
	default:
		return false
	}
}

func isTerminalMessageType(msgType string) bool {
	switch msgType {
	case "AIMessage", "ai", "assistant":
		return true
	default:
		return false
	}
}

// messagesModeAssistantContent extracts plain assistant text from mode="messages"
// events that carry a terminal AIMessage without loop-tagged phase metadata
// (direct_llm turns, including vision attachments).
func (cl *EventClassifier) messagesModeAssistantContent(m soothe.EventMessage) (string, bool) {
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
	if phase, _ := msgMap["phase"].(string); strings.TrimSpace(phase) != "" {
		return "", false
	}
	msgType, _ := msgMap["type"].(string)
	if msgType != "" && !isTerminalMessageType(msgType) {
		return "", false
	}
	content := extractContentFromMessage(msgMap)
	content = strings.TrimSpace(content)
	if content == "" {
		return "", false
	}
	return content, true
}

func firstMessagePayload(data interface{}) (msgType, content, phase string, ok bool) {
	items, ok := data.([]interface{})
	if !ok || len(items) == 0 {
		return "", "", "", false
	}
	msgMap, ok := items[0].(map[string]interface{})
	if !ok {
		return "", "", "", false
	}
	msgType, _ = msgMap["type"].(string)
	phase, _ = msgMap["phase"].(string)
	content = extractContentFromMessage(msgMap)
	return msgType, content, phase, true
}

func extractContentFromMessage(msgMap map[string]interface{}) string {
	if c, ok := msgMap["content"].(string); ok && c != "" {
		return c
	}
	if arr, ok := msgMap["content"].([]interface{}); ok && len(arr) > 0 {
		var b strings.Builder
		for _, item := range arr {
			if s, ok := item.(string); ok {
				b.WriteString(s)
				continue
			}
			if blk, ok := item.(map[string]interface{}); ok {
				if t, ok := blk["text"].(string); ok {
					b.WriteString(t)
				}
			}
		}
		return b.String()
	}
	if blocks, ok := msgMap["content_blocks"].([]interface{}); ok && len(blocks) > 0 {
		var b strings.Builder
		for _, blk := range blocks {
			if m, ok := blk.(map[string]interface{}); ok {
				if t, ok := m["text"].(string); ok {
					b.WriteString(t)
				}
			}
		}
		return b.String()
	}
	return ""
}

// processChatEvent is the event→outcome mapper, ported from triarch's
// ProcessChatEvent with the deliverable phase set made configurable.
func (cl *EventClassifier) processChatEvent(msg interface{}, accumulated string) ChatEventResult {
	switch m := msg.(type) {
	case soothe.EventMessage:
		eventType := m.EventType()
		ns := eventType

		if data, ok := normalizeEventData(m.Data); ok {
			dataType := eventType
			if dt, ok := data["type"].(string); ok && dt != "" {
				dataType = dt
			}
			if dataType == eventLoopHistoryReplayed {
				return ChatEventResult{Terminal: ChatEventContinue}
			}
			if step, ok := cl.extractThinkingStep(dataType, data); ok {
				return thinkingStepResult(step)
			}
		}

		if m.Mode == "messages" {
			msgType, rawContent, phase, hasPayload := firstMessagePayload(m.Data)
			if hasPayload && rawContent != "" && isStreamingMessageType(msgType) {
				return cl.continueResult(rawContent)
			}

			loopMsg, ok := m.LoopAIMessage()
			if ok {
				content := loopMsg.LoopAIText()
				if content != "" {
					if isStreamingMessageType(loopMsg.Type) {
						return cl.continueResult(content)
					}
					if cl.isDeliverableLoopPhase(loopMsg.Phase) && cl.IsSubstantiveAssistantReply(content) {
						return cl.deliverableResult(content, "soothe.protocol.message."+loopMsg.Phase)
					}
					return cl.continueResult(content)
				}
			}
			if content, ok := cl.messagesModeAssistantContent(m); ok && cl.IsSubstantiveAssistantReply(content) {
				return cl.deliverableResult(content, "soothe.protocol.message.direct_model")
			}
			if hasPayload && rawContent != "" {
				if isTerminalMessageType(msgType) || msgType == "" {
					if cl.isDeliverableLoopPhase(phase) && cl.IsSubstantiveAssistantReply(rawContent) {
						return cl.deliverableResult(rawContent, "soothe.protocol.message."+phase)
					}
					return cl.continueResult(rawContent)
				}
				return cl.continueResult(rawContent)
			}
		}

		data, ok := normalizeEventData(m.Data)
		if !ok {
			return ChatEventResult{Terminal: ChatEventContinue}
		}

		var dataType string
		if dt, ok := data["type"].(string); ok {
			dataType = dt
		}
		completionEvent := dataType
		if completionEvent == "" {
			completionEvent = ns
		}

		if isNamespaceMatch(ns, dataType, "soothe.output") || isNamespaceMatch(ns, dataType, "responded") {
			if content, ok := extractContentFromData(data); ok {
				if cl.isFinalOutputEvent(dataType, ns) {
					return cl.deliverableResult(content, completionEvent)
				}
				return cl.continueResult(content)
			}
		}

		if isNamespaceMatch(ns, dataType, "agent_loop.completed") ||
			isNamespaceMatch(ns, dataType, "agent_loop.reasoned") ||
			isNamespaceMatch(ns, dataType, "loop.completed") {
			if content, ok := extractContentFromData(data); ok {
				return cl.continueResult(content)
			}
		}

		if isNamespaceMatch(ns, dataType, "final_report") {
			if content, ok := extractContentFromData(data); ok {
				return cl.deliverableResult(content, completionEvent)
			}
		}

		if strings.Contains(dataType, "soothe.error.") || strings.Contains(ns, "soothe.error.") {
			errType := dataType
			if errType == "" {
				errType = ns
			}
			if msg, ok := data["message"].(string); ok && msg != "" {
				return cl.failedResult(fmt.Errorf("%s: %s", errType, msg))
			}
			if content, ok := extractContentFromData(data); ok {
				return cl.failedResult(fmt.Errorf("%s: %s", errType, content))
			}
			return cl.failedResult(fmt.Errorf("%s", errType))
		}

		if isNamespaceMatch(ns, dataType, "stream") ||
			isNamespaceMatch(ns, dataType, "progress") ||
			isNamespaceMatch(ns, dataType, soothe.EventToolCallUpdatesBatch) ||
			isNamespaceMatch(ns, dataType, soothe.EventStreamToolCallUpdate) {
			if delta, ok := data["delta"].(string); ok {
				return cl.continueResult(delta)
			}
		}

		if isNamespaceMatch(ns, dataType, "heartbeat") ||
			isNamespaceMatch(ns, dataType, "system.daemon") ||
			isNamespaceMatch(ns, dataType, "agent_loop.started") ||
			isNamespaceMatch(ns, dataType, "intent.classified") {
			return ChatEventResult{Terminal: ChatEventContinue}
		}

	case soothe.Envelope:
		// Protocol-1 RPC responses and subscription confirmations arrive as
		// Envelopes. A `response`/`next`/`complete` envelope is a protocol-level
		// ack (not a deliverable); an `error` envelope is a daemon failure.
		switch m.Type {
		case "error":
			code := -32603
			msg := ""
			if m.Error != nil {
				code = m.Error.Code
				msg = m.Error.Message
			}
			return cl.failedResult(fmt.Errorf("daemon error [%d]: %s", code, msg))
		default:
			// response / next / complete / receipt_response — protocol acks.
			return ChatEventResult{Terminal: ChatEventContinue}
		}

	case soothe.ErrorResponse:
		return cl.failedResult(fmt.Errorf("daemon error [%s]: %s", m.Code, m.Message))

	case soothe.StatusResponse:
		log.Printf("[appkit.EventClassifier] Status: state=%s, loopID=%s", m.State, m.LoopID)
		return ChatEventResult{Terminal: ChatEventContinue}

	default:
		log.Printf("[appkit.EventClassifier] unknown message type: %T", msg)
	}

	return ChatEventResult{Terminal: ChatEventContinue}
}

func extractContentFromData(data map[string]interface{}) (string, bool) {
	for _, key := range []string{"final_stdout_message", "completion_summary", "content", "text", "response", "output", "message", "report"} {
		if val, ok := data[key].(string); ok && val != "" {
			return val, true
		}
	}
	if nested, ok := data["data"].(map[string]interface{}); ok {
		for _, key := range []string{"final_stdout_message", "completion_summary", "content", "text", "response", "output", "message", "report"} {
			if val, ok := nested[key].(string); ok && val != "" {
				return val, true
			}
		}
	}
	return "", false
}

func isNamespaceMatch(ns, dataType, pattern string) bool {
	return strings.Contains(dataType, pattern) || strings.Contains(ns, pattern)
}

// isFinalOutputEvent reports soothe output/responded events that carry
// user-facing final text. The phase allowlist is derived from the configured
// DeliverablePhases so the product's deliverable definition stays consistent
// across the classifier and the completion-event check.
func (cl *EventClassifier) isFinalOutputEvent(dataType, ns string) bool {
	combined := dataType + " " + ns
	if strings.Contains(combined, "final_report") {
		return true
	}
	for phase := range cl.cfg.DeliverablePhases {
		if strings.Contains(combined, phase) {
			return true
		}
	}
	return false
}

func normalizeEventData(data interface{}) (map[string]interface{}, bool) {
	if data == nil {
		return nil, false
	}
	if m, ok := data.(map[string]interface{}); ok {
		return m, true
	}
	if s, ok := data.(string); ok {
		var m map[string]interface{}
		if json.Unmarshal([]byte(s), &m) == nil {
			return m, true
		}
		return nil, false
	}
	if raw, ok := data.(json.RawMessage); ok {
		var m map[string]interface{}
		if json.Unmarshal(raw, &m) == nil {
			return m, true
		}
	}
	return nil, false
}
