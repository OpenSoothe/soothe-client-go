package soothe

import (
	"context"
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// High-level API methods (mirroring Python SDK WebSocketClient)
// ---------------------------------------------------------------------------

// SendInput sends user input to the daemon.
func (c *Client) SendInput(ctx context.Context, text string, opts ...InputOption) error {
	o := &inputOptions{autonomous: false}
	for _, opt := range opts {
		opt(o)
	}
	if strings.TrimSpace(o.loopID) == "" {
		return fmt.Errorf("SendInput requires WithLoopID(loopID)")
	}
	payload := map[string]interface{}{
		"type":       "loop_input",
		"loop_id":    strings.TrimSpace(o.loopID),
		"content":    text,
		"autonomous": o.autonomous,
	}
	if o.maxIterations != nil {
		payload["max_iterations"] = *o.maxIterations
	}
	if o.preferredSubagent != "" {
		payload["preferred_subagent"] = o.preferredSubagent
	}
	if o.attachments != nil {
		payload["attachments"] = o.attachments
	}
	if o.interactive {
		payload["interactive"] = true
	}
	if o.model != "" {
		payload["model"] = o.model
	}
	if o.modelParams != nil {
		payload["model_params"] = o.modelParams
	}
	if o.intentHint != "" {
		payload["intent_hint"] = o.intentHint
	}
	if o.responseSchema != nil {
		payload["response_schema"] = o.responseSchema
	}
	if o.responseSchemaName != "" {
		payload["response_schema_name"] = o.responseSchemaName
	}
	if o.responseSchemaStrict != nil {
		payload["response_schema_strict"] = *o.responseSchemaStrict
	}
	// RFC-622 clarification relay
	if o.clarificationMode != "" {
		payload["clarification_mode"] = o.clarificationMode
	}
	if o.clarificationAnswer {
		payload["clarification_answer"] = true
	}
	if o.clarificationAnswers != nil {
		payload["clarification_answers"] = o.clarificationAnswers
	}
	return c.SendMessage(ctx, payload)
}

// InputOption configures an input message.
type InputOption func(*inputOptions)

type inputOptions struct {
	loopID               string
	autonomous           bool
	maxIterations        *int
	preferredSubagent    string
	interactive          bool
	model                string
	modelParams          map[string]interface{}
	attachments          []map[string]interface{}
	intentHint           string
	responseSchema       map[string]interface{}
	responseSchemaName   string
	responseSchemaStrict *bool
	// RFC-622 clarification relay
	clarificationMode    string
	clarificationAnswer  bool
	clarificationAnswers []string
}

// WithLoopID sets the subscribed loop id for loop_input.
func WithLoopID(loopID string) InputOption {
	return func(o *inputOptions) { o.loopID = loopID }
}

// WithAutonomous enables autonomous mode.
func WithAutonomous(maxIterations *int) InputOption {
	return func(o *inputOptions) {
		o.autonomous = true
		o.maxIterations = maxIterations
	}
}

// WithSubagent sets the preferred_subagent hint (mirrors soothe-sdk send_input).
func WithSubagent(name string) InputOption {
	return func(o *inputOptions) { o.preferredSubagent = name }
}

// WithAttachments sets optional image attachments (mime_type + base64 data); see IG-327.
func WithAttachments(attachments []map[string]interface{}) InputOption {
	return func(o *inputOptions) { o.attachments = attachments }
}

// WithInteractive enables interactive mode.
func WithInteractive() InputOption {
	return func(o *inputOptions) { o.interactive = true }
}

// WithModel sets an optional provider:model override.
func WithModel(model string) InputOption {
	return func(o *inputOptions) { o.model = model }
}

// WithModelParams sets extra model parameters.
func WithModelParams(params map[string]interface{}) InputOption {
	return func(o *inputOptions) { o.modelParams = params }
}

// WithIntentHint sets intent_hint on loop_input (daemon normalizes case).
// In-agent bypass value: "quiz".
// Daemon-only direct model value: "direct_llm" (default chat model, or image model when
// WithAttachments is set). Deprecated alias "image_to_text" (attachments required).
func WithIntentHint(hint string) InputOption {
	return func(o *inputOptions) { o.intentHint = hint }
}

// WithResponseSchema sets JSON Schema for strict structured output (direct_llm only).
func WithResponseSchema(schema map[string]interface{}) InputOption {
	return func(o *inputOptions) { o.responseSchema = schema }
}

// WithResponseSchemaName sets the provider schema name for structured direct_llm output.
func WithResponseSchemaName(name string) InputOption {
	return func(o *inputOptions) { o.responseSchemaName = name }
}

// WithResponseSchemaStrict sets whether json_schema strict mode is requested (default true).
func WithResponseSchemaStrict(strict bool) InputOption {
	return func(o *inputOptions) { v := strict; o.responseSchemaStrict = &v }
}

// WithClarificationMode sets RFC-622 clarification relay mode ("auto" / "manual").
func WithClarificationMode(mode string) InputOption {
	return func(o *inputOptions) { o.clarificationMode = mode }
}

// WithClarificationAnswer marks this input as the answer to a pending clarification interrupt.
func WithClarificationAnswer() InputOption {
	return func(o *inputOptions) { o.clarificationAnswer = true }
}

// WithClarificationAnswers sets per-question answers for multi-question clarifications.
func WithClarificationAnswers(answers []string) InputOption {
	return func(o *inputOptions) { o.clarificationAnswers = answers }
}

// SendCommand sends a slash command to the daemon.
func (c *Client) SendCommand(ctx context.Context, cmd string) error {
	return c.SendMessage(ctx, CommandMessage{
		BaseMessage: BaseMessage{Type: "command"},
		Cmd:         cmd,
	})
}

// SendDetach notifies the daemon that this client is detaching.
func (c *Client) SendDetach(ctx context.Context) error {
	return c.SendMessage(ctx, DetachMessage{
		BaseMessage: BaseMessage{Type: "detach"},
	})
}

// SendDaemonReady sends the daemon_ready handshake message.
func (c *Client) SendDaemonReady(ctx context.Context) error {
	return c.SendMessage(ctx, BaseMessage{Type: "daemon_ready"})
}

// SendDaemonStatus requests daemon status check.
func (c *Client) SendDaemonStatus(ctx context.Context, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, DaemonStatusMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "daemon_status"},
	})
}

// SendDaemonShutdown requests daemon shutdown.
func (c *Client) SendDaemonShutdown(ctx context.Context, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, DaemonShutdownMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "daemon_shutdown"},
	})
}

// SendConfigGet requests a config section from the daemon.
func (c *Client) SendConfigGet(ctx context.Context, section string, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, ConfigGetMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "config_get"},
		Section:     section,
	})
}

// SendSkillsList requests the skills catalog (RFC-400).
func (c *Client) SendSkillsList(ctx context.Context, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, SkillsListMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "skills_list"},
	})
}

// SendModelsList requests the models catalog (RFC-400).
func (c *Client) SendModelsList(ctx context.Context, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, ModelsListMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "models_list"},
	})
}

// SendInvokeSkill invokes a skill on the daemon (RFC-400).
func (c *Client) SendInvokeSkill(ctx context.Context, skill, args string, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, InvokeSkillMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "invoke_skill"},
		Skill:       skill,
		Args:        args,
	})
}

// SendCommandRequest sends a structured RPC command (RFC-404).
func (c *Client) SendCommandRequest(ctx context.Context, command string, loopID string, params map[string]interface{}, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, CommandRequestMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "command_request"},
		Command:     command,
		LoopID:      loopID,
		Params:      params,
	})
}

// SendLoopList requests the list of StrangeLoop instances.
func (c *Client) SendLoopList(ctx context.Context, filter map[string]interface{}, limit int, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, LoopListMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "loop_list"},
		Filter:      filter,
		Limit:       limit,
	})
}

// SendLoopGet requests details for a specific loop.
func (c *Client) SendLoopGet(ctx context.Context, loopID string, verbose bool, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, LoopGetMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "loop_get"},
		LoopID:      loopID,
		Verbose:     verbose,
	})
}

// SendLoopTree requests the checkpoint tree for a loop.
func (c *Client) SendLoopTree(ctx context.Context, loopID, format string, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, LoopTreeMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "loop_tree"},
		LoopID:      loopID,
		Format:      format,
	})
}

// SendLoopPrune requests pruning of old branches for a loop.
func (c *Client) SendLoopPrune(ctx context.Context, loopID string, retentionDays int, dryRun bool, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, LoopPruneMessage{
		BaseMessage:   BaseMessage{RequestID: rid, Type: "loop_prune"},
		LoopID:        loopID,
		RetentionDays: retentionDays,
		DryRun:        dryRun,
	})
}

// SendLoopDelete requests deletion of a loop.
func (c *Client) SendLoopDelete(ctx context.Context, loopID string, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, LoopDeleteMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "loop_delete"},
		LoopID:      loopID,
	})
}

// SendLoopReattach requests reattachment to a loop (RFC-411).
func (c *Client) SendLoopReattach(ctx context.Context, loopID string, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, LoopReattachMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "loop_reattach"},
		LoopID:      loopID,
	})
}

// SendLoopSubscribe subscribes to loop events (RFC-503).
// Pass empty verbosity to omit the field (daemon default).
// Pass empty streamDelivery to omit the field (daemon default is "batch").
func (c *Client) SendLoopSubscribe(ctx context.Context, loopID string, verbosity string, streamDelivery string, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, LoopSubscribeMessage{
		BaseMessage:    BaseMessage{RequestID: rid, Type: "loop_subscribe"},
		LoopID:         loopID,
		Verbosity:      verbosity,
		StreamDelivery: streamDelivery,
	})
}

// SendLoopDetach detaches from a loop (RFC-503).
func (c *Client) SendLoopDetach(ctx context.Context, loopID string, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, LoopDetachMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "loop_detach"},
		LoopID:      loopID,
	})
}

// SendLoopNew creates a new loop (RFC-503).
// clientWorkspace is the project directory (used directly when set).
// clientWorkspaceID scopes persisted sandboxes when clientWorkspace is empty.
func (c *Client) SendLoopNew(
	ctx context.Context,
	clientWorkspace string,
	userID string,
	clientWorkspaceID string,
	isEphemeral bool,
	requestID ...string,
) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	msg := LoopNewMessage{
		BaseMessage:       BaseMessage{RequestID: rid, Type: "loop_new"},
		ClientWorkspace:   clientWorkspace,
		UserID:            userID,
		ClientWorkspaceID: clientWorkspaceID,
		IsEphemeral:       isEphemeral,
	}
	return c.SendMessage(ctx, msg)
}

// SendLoopInput sends input to a loop (RFC-503).
func (c *Client) SendLoopInput(ctx context.Context, loopID string, content string, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, LoopInputMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "loop_input"},
		LoopID:      loopID,
		Content:     content,
	})
}

// ---------------------------------------------------------------------------
// Additional loop send methods (RFC-503 extensions)
// ---------------------------------------------------------------------------

// SendLoopMessages requests persisted conversation/activity rows.
func (c *Client) SendLoopMessages(ctx context.Context, loopID string, limit int, offset int, includeEvents bool, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, LoopMessagesMessage{
		BaseMessage:    BaseMessage{RequestID: rid, Type: "loop_messages"},
		LoopID:         loopID,
		Limit:          limit,
		Offset:         offset,
		IncludeEvents:  includeEvents,
	})
}

// SendLoopStateGet requests LangGraph checkpoint channel values.
func (c *Client) SendLoopStateGet(ctx context.Context, loopID string, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, LoopStateGetMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "loop_state_get"},
		LoopID:      loopID,
	})
}

// SendLoopStateUpdate applies partial checkpoint values.
func (c *Client) SendLoopStateUpdate(ctx context.Context, loopID string, values map[string]interface{}, asNode string, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, LoopStateUpdateMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "loop_state_update"},
		LoopID:      loopID,
		Values:      values,
		AsNode:      asNode,
	})
}

// SendLoopCardsFetch requests display card ledger (RFC-413).
func (c *Client) SendLoopCardsFetch(ctx context.Context, loopID string, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, LoopCardsFetchMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "loop_cards_fetch"},
		LoopID:      loopID,
	})
}

// SendMCPStatus requests MCP server status.
func (c *Client) SendMCPStatus(ctx context.Context, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, MCPStatusMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "mcp_status"},
	})
}
