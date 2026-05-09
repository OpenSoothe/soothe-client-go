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
	return c.SendMessage(ctx, payload)
}

// InputOption configures an input message.
type InputOption func(*inputOptions)

type inputOptions struct {
	loopID            string
	autonomous        bool
	maxIterations     *int
	preferredSubagent string
	interactive       bool
	model             string
	modelParams       map[string]interface{}
	attachments       []map[string]interface{}
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

// SendResumeInterrupts sends interactive continuation payload for a paused turn (loop-scoped).
func (c *Client) SendResumeInterrupts(ctx context.Context, loopID string, resumePayload map[string]interface{}, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, ResumeInterruptsMessage{
		BaseMessage:   BaseMessage{RequestID: rid, Type: "resume_interrupts"},
		LoopID:        loopID,
		ResumePayload: resumePayload,
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

// SendLoopList requests the list of AgentLoop instances.
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
func (c *Client) SendLoopSubscribe(ctx context.Context, loopID string, verbosity string, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, LoopSubscribeMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "loop_subscribe"},
		LoopID:      loopID,
		Verbosity:   verbosity,
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
func (c *Client) SendLoopNew(ctx context.Context, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, LoopNewMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "loop_new"},
	})
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
