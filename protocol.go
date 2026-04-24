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

// InputMessage represents user input to the agent.
type InputMessage struct {
	BaseMessage
	Text          string                 `json:"text"`
	ThreadID      string                 `json:"thread_id,omitempty"`
	Autonomous    bool                   `json:"autonomous,omitempty"`
	MaxIterations *int                   `json:"max_iterations,omitempty"`
	Subagent      string                 `json:"subagent,omitempty"`
	Interactive   bool                   `json:"interactive,omitempty"`
	Model         string                 `json:"model,omitempty"`
	ModelParams   map[string]interface{} `json:"model_params,omitempty"`
}

// CommandMessage represents a slash command sent to the daemon.
type CommandMessage struct {
	BaseMessage
	Cmd string `json:"cmd"`
}

// CommandRequestMessage represents a structured RPC command (RFC-404).
type CommandRequestMessage struct {
	BaseMessage
	Command  string                 `json:"command"`
	ThreadID string                 `json:"thread_id,omitempty"`
	Params   map[string]interface{} `json:"params,omitempty"`
}

// SubscribeThreadMessage represents a thread subscription request.
type SubscribeThreadMessage struct {
	BaseMessage
	ThreadID       string `json:"thread_id"`
	VerbosityLevel string `json:"verbosity"`
}

// NewThreadMessage represents new thread creation.
type NewThreadMessage struct {
	BaseMessage
	Workspace string `json:"workspace"`
}

// ResumeThreadMessage represents thread resumption.
type ResumeThreadMessage struct {
	BaseMessage
	ThreadID  string `json:"thread_id"`
	Workspace string `json:"workspace,omitempty"`
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

// ThreadListMessage requests the persisted thread list.
type ThreadListMessage struct {
	BaseMessage
	Filter             map[string]interface{} `json:"filter,omitempty"`
	IncludeStats       bool                   `json:"include_stats,omitempty"`
	IncludeLastMessage bool                   `json:"include_last_message,omitempty"`
}

// ThreadGetMessage requests metadata for a specific thread.
type ThreadGetMessage struct {
	BaseMessage
	ThreadID string `json:"thread_id"`
}

// ThreadMessagesMessage requests paginated thread messages.
type ThreadMessagesMessage struct {
	BaseMessage
	ThreadID      string `json:"thread_id"`
	Limit         int    `json:"limit,omitempty"`
	Offset        int    `json:"offset,omitempty"`
	IncludeEvents bool   `json:"include_events,omitempty"`
}

// ThreadStateMessage requests raw checkpoint state for a thread.
type ThreadStateMessage struct {
	BaseMessage
	ThreadID string `json:"thread_id"`
}

// ThreadUpdateStateMessage persists partial state values for a thread.
type ThreadUpdateStateMessage struct {
	BaseMessage
	ThreadID string                 `json:"thread_id"`
	Values   map[string]interface{} `json:"values"`
}

// ThreadArchiveMessage requests thread archival.
type ThreadArchiveMessage struct {
	BaseMessage
	ThreadID string `json:"thread_id"`
}

// ThreadDeleteMessage requests thread deletion.
type ThreadDeleteMessage struct {
	BaseMessage
	ThreadID string `json:"thread_id"`
}

// ThreadCreateMessage requests creation of a persisted thread (RFC-402).
type ThreadCreateMessage struct {
	BaseMessage
	InitialMessage string                 `json:"initial_message,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// ThreadArtifactsMessage requests thread artifacts (RFC-402).
type ThreadArtifactsMessage struct {
	BaseMessage
	ThreadID string `json:"thread_id"`
}

// ResumeInterruptsMessage sends interactive continuation payload.
type ResumeInterruptsMessage struct {
	BaseMessage
	ThreadID      string                 `json:"thread_id"`
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

// ThreadStatusMessage requests thread runtime status.
type ThreadStatusMessage struct {
	BaseMessage
	ThreadID string `json:"thread_id"`
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
	LoopID string `json:"loop_id"`
}

// LoopDetachMessage detaches from a loop (RFC-503).
type LoopDetachMessage struct {
	BaseMessage
	LoopID string `json:"loop_id"`
}

// LoopNewMessage creates a new loop (RFC-503).
type LoopNewMessage struct {
	BaseMessage
}

// LoopInputMessage sends input to a loop (RFC-503).
type LoopInputMessage struct {
	BaseMessage
	LoopID  string `json:"loop_id"`
	Content string `json:"content"`
}

// ---------------------------------------------------------------------------
// Daemon → Client messages
// ---------------------------------------------------------------------------

// EventMessage represents a streaming event from the agent.
type EventMessage struct {
	BaseMessage
	ThreadID  string                 `json:"thread_id,omitempty"`
	Namespace string                 `json:"namespace"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp,omitempty"`
}

// StatusResponse represents a status acknowledgment.
type StatusResponse struct {
	BaseMessage
	State               string        `json:"state"`
	ThreadID            string        `json:"thread_id"`
	Workspace           string        `json:"workspace"`
	InputHistory        []string      `json:"input_history,omitempty"`
	ConversationHistory []interface{} `json:"conversation_history,omitempty"`
	ThreadResumed       bool          `json:"thread_resumed,omitempty"`
	NewThread           bool          `json:"new_thread,omitempty"`
}

// SubscriptionConfirmedResponse represents a subscription acknowledgment.
type SubscriptionConfirmedResponse struct {
	BaseMessage
	ThreadID  string `json:"thread_id"`
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
	Running       bool `json:"running"`
	PortLive      bool `json:"port_live"`
	ActiveThreads int  `json:"active_threads"`
	DaemonPID     int  `json:"daemon_pid,omitempty"`
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

// ThreadListResponse represents the thread list response.
type ThreadListResponse struct {
	BaseMessage
	Threads []map[string]interface{} `json:"threads,omitempty"`
	Total   int                      `json:"total,omitempty"`
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

// ClearMessage represents a thread cleared notification.
type ClearMessage struct {
	BaseMessage
	ThreadID string `json:"thread_id"`
}

// ThreadCreatedMessage represents a thread creation result.
type ThreadCreatedMessage struct {
	BaseMessage
	ThreadID string `json:"thread_id"`
	Status   string `json:"status"`
}

// ThreadGetResponse represents a thread get result.
type ThreadGetResponse struct {
	BaseMessage
	Thread map[string]interface{} `json:"thread"`
}

// ThreadOperationAckResponse represents thread archive/delete acknowledgment.
type ThreadOperationAckResponse struct {
	BaseMessage
	Operation string `json:"operation"`
	ThreadID  string `json:"thread_id"`
	Success   bool   `json:"success"`
}

// ThreadMessagesResponse represents thread messages result.
type ThreadMessagesResponse struct {
	BaseMessage
	ThreadID string                   `json:"thread_id"`
	Messages []map[string]interface{} `json:"messages,omitempty"`
	Limit    int                      `json:"limit,omitempty"`
	Offset   int                      `json:"offset,omitempty"`
}

// ThreadArtifactsResponse represents thread artifacts result.
type ThreadArtifactsResponse struct {
	BaseMessage
	ThreadID  string                   `json:"thread_id"`
	Artifacts []map[string]interface{} `json:"artifacts,omitempty"`
}

// ThreadStatusResponse represents thread runtime status.
type ThreadStatusResponse struct {
	BaseMessage
	ThreadID       string `json:"thread_id"`
	State          string `json:"state"`
	HasActiveQuery bool   `json:"has_active_query"`
}

// ThreadStateResponse represents raw checkpoint state.
type ThreadStateResponse struct {
	BaseMessage
	ThreadID string                 `json:"thread_id"`
	Values   map[string]interface{} `json:"values"`
}

// ThreadUpdateStateResponse represents state update acknowledgment.
type ThreadUpdateStateResponse struct {
	BaseMessage
	ThreadID string `json:"thread_id"`
	Success  bool   `json:"success"`
}

// InterruptsResumedMessage represents interrupt resume acknowledgment.
type InterruptsResumedMessage struct {
	BaseMessage
	ThreadID string `json:"thread_id"`
	Success  bool   `json:"success"`
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
	LoopID   string `json:"loop_id"`
	ThreadID string `json:"thread_id,omitempty"`
	Success  bool   `json:"success"`
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
	case "input":
		var msg InputMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "command":
		var msg CommandMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "subscribe_thread":
		var msg SubscribeThreadMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "new_thread":
		var msg NewThreadMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "resume_thread":
		var msg ResumeThreadMessage
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

	case "thread_list":
		var msg ThreadListMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "thread_get":
		var msg ThreadGetMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "thread_messages":
		var msg ThreadMessagesMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "thread_state":
		var msg ThreadStateMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "thread_update_state":
		var msg ThreadUpdateStateMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "thread_archive":
		var msg ThreadArchiveMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "thread_delete":
		var msg ThreadDeleteMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "thread_create":
		var msg ThreadCreateMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "thread_artifacts":
		var msg ThreadArtifactsMessage
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

	case "thread_status":
		var msg ThreadStatusMessage
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
		// Some daemon builds emit camelCase for thread id
		if msg.ThreadID == "" {
			var alt struct {
				ThreadID string `json:"threadId"`
			}
			if err := json.Unmarshal(data, &alt); err == nil && alt.ThreadID != "" {
				msg.ThreadID = alt.ThreadID
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

	case "thread_list_response":
		var msg ThreadListResponse
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

	case "clear":
		var msg ClearMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "thread_created":
		var msg ThreadCreatedMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "thread_get_response":
		var msg ThreadGetResponse
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "thread_operation_ack":
		var msg ThreadOperationAckResponse
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "thread_messages_response":
		var msg ThreadMessagesResponse
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "thread_artifacts_response":
		var msg ThreadArtifactsResponse
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "thread_status_response":
		var msg ThreadStatusResponse
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "thread_state_response":
		var msg ThreadStateResponse
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg, nil

	case "thread_update_state_response":
		var msg ThreadUpdateStateResponse
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

// ExtractSootheThreadID returns a non-empty Soothe thread id when present in a daemon message.
func ExtractSootheThreadID(msg interface{}) (string, bool) {
	switch m := msg.(type) {
	case StatusResponse:
		if m.ThreadID != "" {
			return m.ThreadID, true
		}
	case EventMessage:
		if m.ThreadID != "" {
			return m.ThreadID, true
		}
		if m.Data != nil {
			if s, ok := m.Data["thread_id"].(string); ok && s != "" {
				return s, true
			}
			if s, ok := m.Data["threadId"].(string); ok && s != "" {
				return s, true
			}
		}
	case ThreadCreatedMessage:
		if m.ThreadID != "" {
			return m.ThreadID, true
		}
	case ClearMessage:
		if m.ThreadID != "" {
			return m.ThreadID, true
		}
	case ThreadStatusResponse:
		if m.ThreadID != "" {
			return m.ThreadID, true
		}
	case ThreadMessagesResponse:
		if m.ThreadID != "" {
			return m.ThreadID, true
		}
	case ThreadArtifactsResponse:
		if m.ThreadID != "" {
			return m.ThreadID, true
		}
	case ThreadStateResponse:
		if m.ThreadID != "" {
			return m.ThreadID, true
		}
	case ThreadUpdateStateResponse:
		if m.ThreadID != "" {
			return m.ThreadID, true
		}
	case InterruptsResumedMessage:
		if m.ThreadID != "" {
			return m.ThreadID, true
		}
	case LoopInputResponse:
		if m.ThreadID != "" {
			return m.ThreadID, true
		}
	case map[string]interface{}:
		if s, ok := m["thread_id"].(string); ok && s != "" {
			return s, true
		}
		if s, ok := m["threadId"].(string); ok && s != "" {
			return s, true
		}
	}
	return "", false
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

// NewInputMessage creates a new input message with required fields.
func NewInputMessage(text, threadID string) InputMessage {
	return InputMessage{
		BaseMessage: BaseMessage{
			RequestID: uuid.New().String(),
			Type:      "input",
		},
		Text:     text,
		ThreadID: threadID,
	}
}

// NewSubscribeThreadMessage creates a new subscription message.
func NewSubscribeThreadMessage(threadID, verbosity string) SubscribeThreadMessage {
	return SubscribeThreadMessage{
		BaseMessage: BaseMessage{
			RequestID: uuid.New().String(),
			Type:      "subscribe_thread",
		},
		ThreadID:       threadID,
		VerbosityLevel: verbosity,
	}
}

// NewNewThreadMessage creates a new thread message.
func NewNewThreadMessage(workspace string) NewThreadMessage {
	return NewThreadMessage{
		BaseMessage: BaseMessage{
			RequestID: uuid.New().String(),
			Type:      "new_thread",
		},
		Workspace: workspace,
	}
}

// NewResumeThreadMessage creates a resume thread message.
func NewResumeThreadMessage(threadID, workspace string) ResumeThreadMessage {
	return ResumeThreadMessage{
		BaseMessage: BaseMessage{
			RequestID: uuid.New().String(),
			Type:      "resume_thread",
		},
		ThreadID:  threadID,
		Workspace: workspace,
	}
}

// NewRequestID generates a new UUID request ID.
func NewRequestID() string {
	return uuid.New().String()
}
