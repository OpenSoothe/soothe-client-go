package soothe

import "context"

// ---------------------------------------------------------------------------
// High-level API methods (mirroring Python SDK WebSocketClient)
// ---------------------------------------------------------------------------

// SendInput sends user input to the daemon.
func (c *Client) SendInput(ctx context.Context, text string, opts ...InputOption) error {
	o := &inputOptions{autonomous: false}
	for _, opt := range opts {
		opt(o)
	}
	payload := map[string]interface{}{
		"type":        "input",
		"text":        text,
		"autonomous":  o.autonomous,
	}
	if o.maxIterations != nil {
		payload["max_iterations"] = *o.maxIterations
	}
	if o.subagent != "" {
		payload["subagent"] = o.subagent
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
	if o.threadID != "" {
		payload["thread_id"] = o.threadID
	}
	return c.SendMessage(ctx, payload)
}

// InputOption configures an input message.
type InputOption func(*inputOptions)

type inputOptions struct {
	threadID      string
	autonomous    bool
	maxIterations *int
	subagent      string
	interactive   bool
	model         string
	modelParams   map[string]interface{}
}

// WithThreadID sets the thread ID for the input message.
func WithThreadID(threadID string) InputOption {
	return func(o *inputOptions) { o.threadID = threadID }
}

// WithAutonomous enables autonomous mode.
func WithAutonomous(maxIterations *int) InputOption {
	return func(o *inputOptions) {
		o.autonomous = true
		o.maxIterations = maxIterations
	}
}

// WithSubagent routes the query to a specific subagent.
func WithSubagent(name string) InputOption {
	return func(o *inputOptions) { o.subagent = name }
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

// SendNewThread requests the daemon to start a new thread.
func (c *Client) SendNewThread(ctx context.Context, workspace string) error {
	return c.SendMessage(ctx, NewNewThreadMessage(workspace))
}

// SendResumeThread requests the daemon to resume a specific thread.
func (c *Client) SendResumeThread(ctx context.Context, threadID, workspace string) error {
	return c.SendMessage(ctx, NewResumeThreadMessage(threadID, workspace))
}

// SendSubscribeThread subscribes to events for a thread.
func (c *Client) SendSubscribeThread(ctx context.Context, threadID, verbosity string) error {
	return c.SendMessage(ctx, NewSubscribeThreadMessage(threadID, verbosity))
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

// SendThreadList requests the persisted thread list.
func (c *Client) SendThreadList(ctx context.Context, filter map[string]interface{}, includeStats bool, includeLastMessage bool, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, ThreadListMessage{
		BaseMessage:        BaseMessage{RequestID: rid, Type: "thread_list"},
		Filter:             filter,
		IncludeStats:       includeStats,
		IncludeLastMessage: includeLastMessage,
	})
}

// SendThreadGet requests metadata for a specific thread.
func (c *Client) SendThreadGet(ctx context.Context, threadID string, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, ThreadGetMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "thread_get"},
		ThreadID:    threadID,
	})
}

// SendThreadMessages requests paginated thread messages.
func (c *Client) SendThreadMessages(ctx context.Context, threadID string, limit, offset int, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, ThreadMessagesMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "thread_messages"},
		ThreadID:    threadID,
		Limit:       limit,
		Offset:      offset,
	})
}

// SendThreadState requests raw checkpoint state for a thread.
func (c *Client) SendThreadState(ctx context.Context, threadID string, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, ThreadStateMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "thread_state"},
		ThreadID:    threadID,
	})
}

// SendThreadUpdateState persists partial state values for a thread.
func (c *Client) SendThreadUpdateState(ctx context.Context, threadID string, values map[string]interface{}, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, ThreadUpdateStateMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "thread_update_state"},
		ThreadID:    threadID,
		Values:      values,
	})
}

// SendThreadArchive requests thread archival.
func (c *Client) SendThreadArchive(ctx context.Context, threadID string, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, ThreadArchiveMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "thread_archive"},
		ThreadID:    threadID,
	})
}

// SendThreadDelete requests thread deletion.
func (c *Client) SendThreadDelete(ctx context.Context, threadID string, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, ThreadDeleteMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "thread_delete"},
		ThreadID:    threadID,
	})
}

// SendThreadCreate requests creation of a persisted thread (RFC-402).
func (c *Client) SendThreadCreate(ctx context.Context, initialMessage string, metadata map[string]interface{}, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, ThreadCreateMessage{
		BaseMessage:     BaseMessage{RequestID: rid, Type: "thread_create"},
		InitialMessage:  initialMessage,
		Metadata:        metadata,
	})
}

// SendThreadArtifacts requests thread artifacts (RFC-402).
func (c *Client) SendThreadArtifacts(ctx context.Context, threadID string, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, ThreadArtifactsMessage{
		BaseMessage: BaseMessage{RequestID: rid, Type: "thread_artifacts"},
		ThreadID:    threadID,
	})
}

// SendResumeInterrupts sends interactive continuation payload for a paused thread.
func (c *Client) SendResumeInterrupts(ctx context.Context, threadID string, resumePayload map[string]interface{}, requestID ...string) error {
	rid := ""
	if len(requestID) > 0 {
		rid = requestID[0]
	} else {
		rid = NewRequestID()
	}
	return c.SendMessage(ctx, ResumeInterruptsMessage{
		BaseMessage:    BaseMessage{RequestID: rid, Type: "resume_interrupts"},
		ThreadID:       threadID,
		ResumePayload:  resumePayload,
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