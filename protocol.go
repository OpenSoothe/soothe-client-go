package soothe

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Base types
// ---------------------------------------------------------------------------

// BaseMessage represents the common message structure with type and optional request_id.
type BaseMessage struct {
	RequestID string `json:"request_id,omitempty"`
	Type      string `json:"type"`
}

// ---------------------------------------------------------------------------
// Client → Daemon messages
// ---------------------------------------------------------------------------

// CommandMessage represents a slash command sent to the daemon.
type CommandMessage struct {
	BaseMessage
	Cmd string `json:"cmd"`
}

// CommandRequestMessage represents a structured RPC command (RFC-404).
type CommandRequestMessage struct {
	BaseMessage
	Command string                 `json:"command"`
	LoopID  string                 `json:"loop_id,omitempty"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

// DaemonStatusMessage requests daemon status.
type DaemonStatusMessage struct {
	BaseMessage
}

// DaemonShutdownMessage requests daemon shutdown.
type DaemonShutdownMessage struct {
	BaseMessage
}

// ConfigGetMessage requests a config section.
type ConfigGetMessage struct {
	BaseMessage
	Section string `json:"section"`
}

// ResumeInterruptsMessage sends interactive continuation payload.
type ResumeInterruptsMessage struct {
	BaseMessage
	LoopID        string                 `json:"loop_id"`
	ResumePayload map[string]interface{} `json:"resume_payload"`
}

// SkillsListMessage requests the skills catalog (RFC-400).
type SkillsListMessage struct {
	BaseMessage
}

// ModelsListMessage requests the models catalog (RFC-400).
type ModelsListMessage struct {
	BaseMessage
}

// InvokeSkillMessage invokes a skill on the daemon (RFC-400).
type InvokeSkillMessage struct {
	BaseMessage
	Skill string `json:"skill"`
	Args  string `json:"args,omitempty"`
}

// DetachMessage notifies the daemon that the client is detaching.
type DetachMessage struct {
	BaseMessage
}

// LoopListMessage requests the list of AgentLoop instances.
type LoopListMessage struct {
	BaseMessage
	Filter map[string]interface{} `json:"filter,omitempty"`
	Limit  int                    `json:"limit,omitempty"`
}

// LoopGetMessage requests details for a specific loop.
type LoopGetMessage struct {
	BaseMessage
	LoopID  string `json:"loop_id"`
	Verbose bool   `json:"verbose,omitempty"`
}

// LoopTreeMessage requests the checkpoint tree for a loop.
type LoopTreeMessage struct {
	BaseMessage
	LoopID string `json:"loop_id"`
	Format string `json:"format,omitempty"`
}

// LoopPruneMessage requests pruning of old branches for a loop.
type LoopPruneMessage struct {
	BaseMessage
	LoopID        string `json:"loop_id"`
	RetentionDays int    `json:"retention_days,omitempty"`
	DryRun        bool   `json:"dry_run,omitempty"`
}

// LoopDeleteMessage requests deletion of a loop.
type LoopDeleteMessage struct {
	BaseMessage
	LoopID string `json:"loop_id"`
}

// LoopReattachMessage requests reattachment to a loop (RFC-411).
type LoopReattachMessage struct {
	BaseMessage
	LoopID string `json:"loop_id"`
}

// LoopSubscribeMessage subscribes to loop events (RFC-503).
type LoopSubscribeMessage struct {
	BaseMessage
	LoopID         string `json:"loop_id"`
	Verbosity      string `json:"verbosity,omitempty"`
	StreamDelivery string `json:"stream_delivery,omitempty"` // "batch" or "streaming" (RFC-614)
}

// LoopDetachMessage detaches from a loop (RFC-503).
type LoopDetachMessage struct {
	BaseMessage
	LoopID string `json:"loop_id"`
}

// LoopNewMessage creates a new loop (RFC-503).
type LoopNewMessage struct {
	BaseMessage
	Workspace string `json:"workspace,omitempty"` // Client CWD for file/shell tools (IG-409)
}

// LoopInputMessage sends input to a loop (RFC-503).
type LoopInputMessage struct {
	BaseMessage
	LoopID            string                   `json:"loop_id"`
	Content           string                   `json:"content"`
	Autonomous        bool                     `json:"autonomous,omitempty"`
	MaxIterations     *int                     `json:"max_iterations,omitempty"`
	PreferredSubagent string                   `json:"preferred_subagent,omitempty"`
	Interactive       bool                     `json:"interactive,omitempty"`
	Model             string                   `json:"model,omitempty"`
	ModelParams       map[string]interface{}   `json:"model_params,omitempty"`
	Attachments       []map[string]interface{} `json:"attachments,omitempty"`
}

// ---------------------------------------------------------------------------
// Daemon → Client messages
// ---------------------------------------------------------------------------

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

// SubscriptionConfirmedResponse represents a subscription acknowledgment.
type SubscriptionConfirmedResponse struct {
	BaseMessage
	LoopID    string `json:"loop_id,omitempty"`
	ClientID  string `json:"client_id"`
	Verbosity string `json:"verbosity"`
}

// ErrorResponse represents an error message from the daemon.
type ErrorResponse struct {
	BaseMessage
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// DaemonReadyResponse represents daemon readiness.
type DaemonReadyResponse struct {
	BaseMessage
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
}

// DaemonStatusResponse represents the daemon status response.
type DaemonStatusResponse struct {
	BaseMessage
	Running     bool `json:"running"`
	PortLive    bool `json:"port_live"`
	ActiveLoops int  `json:"active_loops"`
	DaemonPID   int  `json:"daemon_pid,omitempty"`
}

// ShutdownAckResponse represents the daemon shutdown acknowledgment.
type ShutdownAckResponse struct {
	BaseMessage
	Status string `json:"status"`
}

// ConfigGetResponse represents the config section response.
type ConfigGetResponse struct {
	BaseMessage
	// Section data is in extra fields; we use raw map for flexibility.
}

// SkillsListResponse represents the skills list response.
type SkillsListResponse struct {
	BaseMessage
	Skills []map[string]interface{} `json:"skills,omitempty"`
}

// ModelsListResponse represents the models list response.
type ModelsListResponse struct {
	BaseMessage
	Models []map[string]interface{} `json:"models,omitempty"`
}

// InvokeSkillResponse represents the skill invocation response.
type InvokeSkillResponse struct {
	BaseMessage
	Echo map[string]interface{} `json:"echo,omitempty"`
}

// CommandResponseMessage represents an RPC command response (RFC-404).
type CommandResponseMessage struct {
	BaseMessage
	Command string                 `json:"command"`
	Data    map[string]interface{} `json:"data,omitempty"`
	Error   string                 `json:"error,omitempty"`
}

// InterruptsResumedMessage represents interrupt resume acknowledgment.
type InterruptsResumedMessage struct {
	BaseMessage
	LoopID  string `json:"loop_id"`
	Success bool   `json:"success"`
}

// LoopListResponse represents the loop list response.
type LoopListResponse struct {
	BaseMessage
	Loops []map[string]interface{} `json:"loops,omitempty"`
	Total int                      `json:"total,omitempty"`
}

// LoopGetResponse represents loop details response.
type LoopGetResponse struct {
	BaseMessage
	Loop map[string]interface{} `json:"loop"`
}

// LoopTreeResponse represents checkpoint tree response.
type LoopTreeResponse struct {
	BaseMessage
	Tree map[string]interface{} `json:"tree"`
}

// LoopPruneResponse represents prune result response.
type LoopPruneResponse struct {
	BaseMessage
	Result map[string]interface{} `json:"result"`
}

// LoopDeleteResponse represents loop delete response.
type LoopDeleteResponse struct {
	BaseMessage
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// LoopSubscribeResponse represents loop subscription result.
type LoopSubscribeResponse struct {
	BaseMessage
	LoopID  string `json:"loop_id"`
	Success bool   `json:"success"`
}

// LoopDetachResponse represents loop detach result.
type LoopDetachResponse struct {
	BaseMessage
	LoopID  string `json:"loop_id"`
	Success bool   `json:"success"`
}

// LoopNewResponse represents new loop creation result.
type LoopNewResponse struct {
	BaseMessage
	LoopID  string `json:"loop_id"`
	Success bool   `json:"success"`
}

// LoopInputResponse represents loop input result.
type LoopInputResponse struct {
	BaseMessage
	LoopID  string `json:"loop_id"`
	Success bool   `json:"success"`
}

// ---------------------------------------------------------------------------
// Encode / Decode
// ---------------------------------------------------------------------------

// EncodeMessage encodes a message as JSON with newline delimiter.
func EncodeMessage(msg interface{}) ([]byte, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// DecodeMessage decodes a JSON message and returns a typed Go struct.
// Unknown types are returned as map[string]interface{}.
func DecodeMessage(data []byte) (interface{}, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var base BaseMessage
	if err := json.Unmarshal(data, &base); err != nil {
		return nil, err
	}

	switch base.Type {
	case "command":
		var msg CommandMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "daemon_status":
		var msg DaemonStatusMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "daemon_shutdown":
		var msg DaemonShutdownMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "config_get":
		var msg ConfigGetMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "resume_interrupts":
		var msg ResumeInterruptsMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "skills_list":
		var msg SkillsListMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "models_list":
		var msg ModelsListMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "invoke_skill":
		var msg InvokeSkillMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "detach":
		var msg DetachMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "command_request":
		var msg CommandRequestMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "loop_list":
		var msg LoopListMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "loop_get":
		var msg LoopGetMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "loop_tree":
		var msg LoopTreeMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "loop_prune":
		var msg LoopPruneMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "loop_delete":
		var msg LoopDeleteMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "loop_reattach":
		var msg LoopReattachMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "loop_subscribe":
		var msg LoopSubscribeMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "loop_detach":
		var msg LoopDetachMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "loop_new":
		var msg LoopNewMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "loop_input":
		var msg LoopInputMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	// Daemon → Client message types
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

	case "subscription_confirmed":
		var msg SubscriptionConfirmedResponse
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "error":
		var msg ErrorResponse
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "daemon_ready":
		var msg DaemonReadyResponse
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "daemon_status_response":
		var msg DaemonStatusResponse
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "shutdown_ack":
		var msg ShutdownAckResponse
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "config_get_response":
		var msg map[string]interface{}
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "skills_list_response":
		var msg SkillsListResponse
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "models_list_response":
		var msg ModelsListResponse
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "invoke_skill_response":
		var msg InvokeSkillResponse
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "command_response":
		var msg CommandResponseMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "interrupts_resumed":
		var msg InterruptsResumedMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "loop_list_response":
		var msg LoopListResponse
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "loop_get_response":
		var msg LoopGetResponse
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "loop_tree_response":
		var msg LoopTreeResponse
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "loop_prune_response":
		var msg LoopPruneResponse
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "loop_delete_response":
		var msg LoopDeleteResponse
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "loop_subscribe_response":
		var msg LoopSubscribeResponse
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "loop_detach_response":
		var msg LoopDetachResponse
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "loop_new_response":
		var msg LoopNewResponse
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "loop_input_response":
		var msg LoopInputResponse
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	default:
		// Unknown type, return as generic map
		var msg map[string]interface{}
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil
	}
}

// ExtractSootheLoopID returns a non-empty loop or checkpoint id when present in a daemon message.
func ExtractSootheLoopID(msg interface{}) (string, bool) {
	switch m := msg.(type) {
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
	case InterruptsResumedMessage:
		if m.LoopID != "" {
			return m.LoopID, true
		}
	case LoopInputResponse:
		if m.LoopID != "" {
			return m.LoopID, true
		}
	case map[string]interface{}:
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
	case "goal_completion", "quiz", "autonomous_goal", "direct_model":
		return true
	default:
		return false
	}
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

// SplitSootheWirePayload returns one or more JSON objects from a single WebSocket text payload.
// The daemon may send newline-delimited JSON (NDJSON) in one frame.
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

// DecodeStream decodes newline-delimited JSON stream.
func DecodeStream(reader io.Reader) (<-chan interface{}, error) {
	ch := make(chan interface{}, 100)

	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			msg, err := DecodeMessage(line)
			if err != nil {
				continue
			}
			if msg != nil {
				ch <- msg
			}
		}
	}()

	return ch, nil
}

// ---------------------------------------------------------------------------
// Message factory functions
// ---------------------------------------------------------------------------

// NewRequestID generates a new UUID request ID.
func NewRequestID() string {
	return uuid.New().String()
}