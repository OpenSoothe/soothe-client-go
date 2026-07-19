package soothe

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ProtoVersion is the protocol-1 version string.
const ProtoVersion = "1"

// DefaultClientCapabilities are declared in the connection_init handshake.
var DefaultClientCapabilities = []string{"streaming", "batch", "heartbeat", "receipts"}

// ClientVersion reported in the connection_init handshake.
const ClientVersion = "0.4.5"

// ---------------------------------------------------------------------------
// Protocol-1 wire envelope
// ---------------------------------------------------------------------------

// Envelope is the unified {proto, type, method, params, id} base structure.
// Field tags use the protocol-1 wire names. Optional fields use omitempty so
// the wire form is compact.
type Envelope struct {
	Proto   string                 `json:"proto"`
	Type    string                 `json:"type"`
	Method  string                 `json:"method,omitempty"`
	Params  map[string]interface{} `json:"params,omitempty"`
	ID      string                 `json:"id,omitempty"`
	Result  map[string]interface{} `json:"result,omitempty"`
	Error   *ErrorObject           `json:"error,omitempty"`
	Payload map[string]interface{} `json:"payload,omitempty"`
	Receipt string                 `json:"receipt,omitempty"`
}

// ErrorObject is the structured error nested under envelope.error.
type ErrorObject struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

// NewRequestEnvelope builds a request envelope with a generated id.
func NewRequestEnvelope(method string, params map[string]interface{}) Envelope {
	return Envelope{
		Proto:  ProtoVersion,
		Type:   "request",
		Method: method,
		Params: params,
		ID:     NewRequestID(),
	}
}

// NewRequestEnvelopeWithID builds a request envelope with an explicit id.
func NewRequestEnvelopeWithID(method string, params map[string]interface{}, id string) Envelope {
	if id == "" {
		id = NewRequestID()
	}
	return Envelope{Proto: ProtoVersion, Type: "request", Method: method, Params: params, ID: id}
}

// NewNotificationEnvelope builds a fire-and-forget notification (no id).
func NewNotificationEnvelope(method string, params map[string]interface{}) Envelope {
	return Envelope{Proto: ProtoVersion, Type: "notification", Method: method, Params: params}
}

// NewSubscribeEnvelope builds a subscribe envelope with a generated id.
func NewSubscribeEnvelope(method string, params map[string]interface{}) Envelope {
	return Envelope{Proto: ProtoVersion, Type: "subscribe", Method: method, Params: params, ID: NewRequestID()}
}

// NewUnsubscribeEnvelope builds an unsubscribe envelope by subscription id.
func NewUnsubscribeEnvelope(id string) Envelope {
	return Envelope{Proto: ProtoVersion, Type: "unsubscribe", ID: id}
}

// NewConnectionInitEnvelope builds the connection_init handshake message.
func NewConnectionInitEnvelope() Envelope {
	return Envelope{
		Proto: ProtoVersion,
		Type:  "connection_init",
		Params: map[string]interface{}{
			"client_version": ClientVersion,
			"client_name":    "soothe-client-go",
			"accept_proto":   []string{ProtoVersion},
			"capabilities":   DefaultClientCapabilities,
		},
	}
}

// NewPingEnvelope builds a ping heartbeat message.
func NewPingEnvelope() Envelope {
	return Envelope{Proto: ProtoVersion, Type: "ping"}
}

// NewPongEnvelope builds a pong heartbeat message.
func NewPongEnvelope() Envelope {
	return Envelope{Proto: ProtoVersion, Type: "pong"}
}

// NewDisconnectEnvelope builds a disconnect notification.
func NewDisconnectEnvelope() Envelope {
	return Envelope{Proto: ProtoVersion, Type: "disconnect"}
}

// BaseMessage represents the common message structure with type and optional request_id.
type BaseMessage struct {
	RequestID string `json:"request_id,omitempty"`
	Type      string `json:"type"`
}

// EventMessage represents a streaming event from the agent.
type EventMessage struct {
	BaseMessage
	LoopID    string      `json:"loop_id,omitempty"`
	Namespace interface{} `json:"namespace,omitempty"`
	Mode      string      `json:"mode,omitempty"`
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp,omitempty"`
}

// LoopAIMessage represents a loop-tagged assistant payload forwarded on
// mode="messages" streams.
type LoopAIMessage struct {
	Type    string      `json:"type,omitempty"`
	Content interface{} `json:"content,omitempty"`
	Phase   string      `json:"phase,omitempty"`
}

// StatusResponse represents a status acknowledgment.
type StatusResponse struct {
	BaseMessage
	State               string        `json:"state"`
	LoopID              string        `json:"loop_id,omitempty"`
	Workspace           string        `json:"workspace"`
	InputHistory        []string      `json:"input_history,omitempty"`
	ConversationHistory []interface{} `json:"conversation_history,omitempty"`
}

// ErrorResponse represents an error message from the daemon.
type ErrorResponse struct {
	BaseMessage
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// ---------------------------------------------------------------------------
// Encode / Decode
// ---------------------------------------------------------------------------

// DecodeMessage decodes a JSON message and returns a typed Go struct.
//
// Protocol-1 envelopes are decoded into the unified Envelope struct. Legacy
// flat-form frames (status, event, error) are decoded into their typed structs
// for backward compatibility with test harnesses. Unknown types are returned
// as map[string]interface{}.
func DecodeMessage(data []byte) (interface{}, error) {
	if len(data) == 0 {
		return nil, nil
	}

	// First peek at the type to dispatch.
	var probe struct {
		Proto string `json:"proto"`
		Type  string `json:"type"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, err
	}

	// Protocol-1 envelope types. Decode into the unified
	// Envelope struct so callers can inspect type/method/id/result/error/payload.
	switch probe.Type {
	case "connection_init", "connection_ack",
		"request", "response", "notification", "subscribe",
		"error", "complete", "unsubscribe",
		"ping", "pong", "receipt_response", "disconnect":
		var env Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			return nil, err
		}
		return env, nil
	case "next":
		// `next` frames carry subscription stream events. The daemon wraps
		// legacy free-form frames as {payload:{namespace, mode, data}}. Project
		// event-shaped payloads to EventMessage so consumers using
		// `raw.(EventMessage)` (LoopAIMessage, NamespaceParts, etc.) keep
		// working. Non-event next frames (e.g. subscription confirmation
		// payload.event=="subscribed") stay as Envelope.
		var env Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			return nil, err
		}
		if em, ok := nextToEventMessage(env); ok {
			return em, nil
		}
		return env, nil
	}

	// Legacy / pass-through types.
	switch probe.Type {
	case "event":
		var msg EventMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil
	case "status":
		var msg StatusResponse
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		// Some daemon builds emit camelCase ids
		if msg.LoopID == "" {
			var altLoop struct {
				LoopID string `json:"loopId"`
			}
			if err := json.Unmarshal(data, &altLoop); err == nil && altLoop.LoopID != "" {
				msg.LoopID = altLoop.LoopID
			}
		}
		return msg, nil
	case "event_batch":
		var msg map[string]interface{}
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil
	default:
		// Unknown type, return as generic map.
		var msg map[string]interface{}
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil
	}
}

// nextToEventMessage projects a protocol-1 `next` envelope's payload into an
// EventMessage when the payload carries a wrapped legacy event frame.
//
// The daemon wraps legacy free-form frames as
// {payload:{namespace, mode:<orig type>, data:<orig frame>}}.
// The original event frame (with its own type/mode/data/loop_id) lives inside
// payload.data, so we project from the inner frame to preserve the legacy
// EventMessage shape (Mode, Data, LoopID, Namespace) that consumers expect.
// Returns (EventMessage, true) when the payload wraps an event-shaped frame;
// otherwise (zero, false) so the caller keeps the raw Envelope (e.g. for
// subscription-confirmation next frames whose payload is {event:"subscribed"}).
func nextToEventMessage(env Envelope) (EventMessage, bool) {
	var zero EventMessage
	payload := env.Payload
	if payload == nil {
		return zero, false
	}

	// Common case: payload.data is the original event frame
	// {type:"event"|"status"|..., mode:"messages"|..., data:[...], loop_id:...}.
	if inner, ok := payload["data"].(map[string]interface{}); ok {
		innerType, _ := inner["type"].(string)
		// Only project event-like frames; status frames are top-level protocol-1
		// types and are not wrapped in next (but defend anyway).
		if innerType == "event" || innerType == "" {
			if _, hasMode := inner["mode"]; hasMode {
				em := EventMessage{
					BaseMessage: BaseMessage{Type: "event"},
					Mode:        asString(inner["mode"]),
					Namespace:   inner["namespace"],
					Data:        inner["data"],
				}
				if lid, ok := inner["loop_id"].(string); ok && lid != "" {
					em.LoopID = lid
				}
				return em, true
			}
		}
	}

	// Fallback: payload itself carries mode/data directly (some daemon paths).
	mode, hasMode := payload["mode"].(string)
	_, hasData := payload["data"]
	if hasMode || hasData {
		em := EventMessage{
			BaseMessage: BaseMessage{Type: "event"},
			Mode:        mode,
			Namespace:   payload["namespace"],
			Data:        payload["data"],
		}
		if lid, ok := payload["loop_id"].(string); ok && lid != "" {
			em.LoopID = lid
		}
		return em, true
	}
	return zero, false
}

// ExpandWireMessages flattens daemon "event_batch" envelopes into individual
// wire messages. Under protocol-1 the batch wrapper is preserved as a
// transport-level optimization; each sub-event is individually wrapped as a
// `next` envelope by the daemon, so callers must expand before dispatching.
func ExpandWireMessages(msg interface{}) []interface{} {
	if msg == nil {
		return nil
	}
	m, ok := msg.(map[string]interface{})
	if !ok {
		return []interface{}{msg}
	}
	typ, _ := m["type"].(string)
	if typ != "event_batch" {
		return []interface{}{msg}
	}
	rawEvents, ok := m["events"].([]interface{})
	if !ok || len(rawEvents) == 0 {
		return nil
	}
	out := make([]interface{}, 0, len(rawEvents))
	for _, sub := range rawEvents {
		subMap, ok := sub.(map[string]interface{})
		if !ok {
			continue
		}
		raw, err := json.Marshal(subMap)
		if err != nil {
			continue
		}
		decoded, err := DecodeMessage(raw)
		if err != nil || decoded == nil {
			continue
		}
		out = append(out, decoded)
	}
	return out
}

// ExtractSootheLoopID returns a non-empty loop or checkpoint id when present in
// a daemon message. Handles both protocol-1 envelopes (next.payload.data.loop_id,
// status.loop_id) and legacy frames.
func ExtractSootheLoopID(msg interface{}) (string, bool) {
	switch m := msg.(type) {
	case Envelope:
		if m.Type == "next" {
			if payload, ok := m.Payload["data"].(map[string]interface{}); ok && payload != nil {
				if s, ok := payload["loop_id"].(string); ok && s != "" {
					return s, true
				}
				if s, ok := payload["loopId"].(string); ok && s != "" {
					return s, true
				}
			}
			if s, ok := m.Payload["loop_id"].(string); ok && s != "" {
				return s, true
			}
			return "", false
		}
		if m.Type == "status" {
			if s, ok := m.Params["loop_id"].(string); ok && s != "" {
				return s, true
			}
			return "", false
		}
	case StatusResponse:
		if m.LoopID != "" {
			return m.LoopID, true
		}
	case EventMessage:
		if m.LoopID != "" {
			return m.LoopID, true
		}
		if dataMap, ok := m.Data.(map[string]interface{}); ok && dataMap != nil {
			if s, ok := dataMap["loop_id"].(string); ok && s != "" {
				return s, true
			}
			if s, ok := dataMap["loopId"].(string); ok && s != "" {
				return s, true
			}
		}
	case map[string]interface{}:
		if t, _ := m["type"].(string); t == "next" {
			if payload, ok := m["payload"].(map[string]interface{}); ok {
				if data, ok := payload["data"].(map[string]interface{}); ok && data != nil {
					if s, ok := data["loop_id"].(string); ok && s != "" {
						return s, true
					}
					if s, ok := data["loopId"].(string); ok && s != "" {
						return s, true
					}
				}
				if s, ok := payload["loop_id"].(string); ok && s != "" {
					return s, true
				}
			}
			return "", false
		}
		if s, ok := m["loop_id"].(string); ok && s != "" {
			return s, true
		}
		if s, ok := m["loopId"].(string); ok && s != "" {
			return s, true
		}
	}
	return "", false
}

// EventType returns the normalized event type for custom events and legacy
// namespace-based events.
func (e EventMessage) EventType() string {
	if m, ok := e.Data.(map[string]interface{}); ok {
		if t, ok := m["type"].(string); ok && t != "" {
			return t
		}
	}
	if s, ok := e.Namespace.(string); ok && s != "" {
		return s
	}
	return ""
}

// NamespaceParts returns namespace path segments regardless of whether the wire
// payload encoded namespace as string or list.
func (e EventMessage) NamespaceParts() []string {
	switch ns := e.Namespace.(type) {
	case []string:
		return ns
	case []interface{}:
		parts := make([]string, 0, len(ns))
		for _, p := range ns {
			if s, ok := p.(string); ok {
				parts = append(parts, s)
			}
		}
		return parts
	case string:
		if ns == "" {
			return nil
		}
		return strings.Split(ns, ".")
	default:
		return nil
	}
}

// LoopAIMessage extracts loop-tagged assistant messages from mode="messages"
// events. Returns false for non-message events or non-assistant payloads.
func (e EventMessage) LoopAIMessage() (LoopAIMessage, bool) {
	var zero LoopAIMessage
	if e.Mode != "messages" {
		return zero, false
	}
	items, ok := e.Data.([]interface{})
	if !ok || len(items) == 0 {
		return zero, false
	}
	msgMap, ok := items[0].(map[string]interface{})
	if !ok {
		return zero, false
	}

	var msg LoopAIMessage
	if t, _ := msgMap["type"].(string); t != "" {
		msg.Type = t
	}
	msg.Content = msgMap["content"]
	if phase, _ := msgMap["phase"].(string); phase != "" {
		msg.Phase = phase
	}
	if msg.Phase == "" {
		return zero, false
	}
	if !isLoopAssistantPhase(msg.Phase) {
		return zero, false
	}
	if msg.Type == "" {
		msg.Type = "ai"
	}
	return msg, true
}

func isLoopAssistantPhase(phase string) bool {
	switch phase {
	case "goal_completion", "quiz", "autonomous_goal",
		"text_completion", "image_to_text", "ocr", "embed", "plan_direct",
		"chitchat":
		return true
	default:
		return false
	}
}

// IsLoopAssistantPhase reports whether phase tags streamable loop assistant
// output (including plan_direct narration). This is broader than
// DefaultDeliverablePhases, which only lists phases that may end a turn.
func IsLoopAssistantPhase(phase string) bool {
	return isLoopAssistantPhase(phase)
}

// LoopAIText extracts plain text from loop-tagged assistant payload content.
func (m LoopAIMessage) LoopAIText() string {
	switch c := m.Content.(type) {
	case string:
		return c
	case []interface{}:
		var b strings.Builder
		for _, item := range c {
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
	default:
		if blk, ok := c.(map[string]interface{}); ok {
			if t, ok := blk["text"].(string); ok {
				return t
			}
		}
		return ""
	}
}

// SplitSootheWirePayload returns one or more JSON objects from a single
// WebSocket text payload. The daemon may send newline-delimited JSON (NDJSON)
// in one frame.
func SplitSootheWirePayload(data []byte) [][]byte {
	s := strings.TrimSpace(string(data))
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	out := make([][]byte, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, []byte(line))
	}
	if len(out) == 0 {
		return [][]byte{data}
	}
	return out
}

// ---------------------------------------------------------------------------
// Message factory functions
// ---------------------------------------------------------------------------

// NewRequestID generates a new UUID request correlation id.
func NewRequestID() string {
	return uuid.New().String()
}
