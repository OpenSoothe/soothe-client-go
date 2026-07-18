package soothe

import (
	"context"
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// High-level API methods (mirroring Python SDK WebSocketClient, protocol-1)
// ---------------------------------------------------------------------------

// SendInput sends user input to the daemon as a loop_input notification.
func (c *Client) SendInput(ctx context.Context, text string, opts ...InputOption) error {
	o := &inputOptions{autonomous: false}
	for _, opt := range opts {
		opt(o)
	}
	if strings.TrimSpace(o.loopID) == "" {
		return fmt.Errorf("SendInput requires WithLoopID(loopID)")
	}
	params := map[string]interface{}{
		"loop_id":    strings.TrimSpace(o.loopID),
		"content":    text,
		"autonomous": o.autonomous,
	}
	if o.maxIterations != nil {
		params["max_iterations"] = *o.maxIterations
	}
	if o.preferredSubagent != "" {
		params["preferred_subagent"] = o.preferredSubagent
	}
	if o.attachments != nil {
		params["attachments"] = o.attachments
	}
	if o.model != "" {
		params["model"] = o.model
	}
	if o.modelParams != nil {
		params["model_params"] = o.modelParams
	}
	if o.intentHint != "" {
		if err := ValidateLoopInputIntentHint(o.intentHint); err != nil {
			return err
		}
		params["intent_hint"] = o.intentHint
	}
	if o.responseSchema != nil {
		params["response_schema"] = o.responseSchema
	}
	if o.responseSchemaName != "" {
		params["response_schema_name"] = o.responseSchemaName
	}
	if o.responseSchemaStrict != nil {
		params["response_schema_strict"] = *o.responseSchemaStrict
	}
	if o.clarificationMode != "" {
		params["clarification_mode"] = o.clarificationMode
	}
	if o.clarificationAnswer {
		params["clarification_answer"] = true
	}
	if o.clarificationAnswers != nil {
		params["clarification_answers"] = o.clarificationAnswers
	}
	return c.SendMessage(ctx, NewNotificationEnvelope("loop_input", params))
}

// InputOption configures an input message.
type InputOption func(*inputOptions)

type inputOptions struct {
	loopID               string
	autonomous           bool
	maxIterations        *int
	preferredSubagent    string
	model                string
	modelParams          map[string]interface{}
	attachments          []map[string]interface{}
	intentHint           string
	responseSchema       map[string]interface{}
	responseSchemaName   string
	responseSchemaStrict *bool
	clarificationMode    string
	clarificationAnswer  bool
	clarificationAnswers []string
}

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

// WithSubagent sets the preferred_subagent hint.
func WithSubagent(name string) InputOption {
	return func(o *inputOptions) { o.preferredSubagent = name }
}

// WithAttachments sets optional image attachments (mime_type + base64 data).
func WithAttachments(attachments []map[string]interface{}) InputOption {
	return func(o *inputOptions) { o.attachments = attachments }
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
// For intent-hint turns use IntentHintTextCompletion, IntentHintImageToText,
// IntentHintOCR, or IntentHintEmbed. Agent-path pass-through hints
// (e.g. resume_clarification, skill:foo) are also accepted. Legacy values
// direct_llm, quiz, and direct_model are rejected before send.
func WithIntentHint(hint string) InputOption {
	return func(o *inputOptions) { o.intentHint = hint }
}

// WithResponseSchema sets JSON Schema for structured output (text_completion or image_to_text).
func WithResponseSchema(schema map[string]interface{}) InputOption {
	return func(o *inputOptions) { o.responseSchema = schema }
}

// WithResponseSchemaName sets the provider schema name for structured output.
func WithResponseSchemaName(name string) InputOption {
	return func(o *inputOptions) { o.responseSchemaName = name }
}

// WithResponseSchemaStrict sets whether json_schema strict mode is requested (default true).
func WithResponseSchemaStrict(strict bool) InputOption {
	return func(o *inputOptions) { v := strict; o.responseSchemaStrict = &v }
}

// WithClarificationMode sets clarification relay mode ("auto" / "manual").
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

// SendCommand sends a slash command to the daemon (slash_command notification).
func (c *Client) SendCommand(ctx context.Context, cmd string) error {
	return c.SendMessage(ctx, NewNotificationEnvelope("slash_command", map[string]interface{}{"cmd": cmd}))
}

// SendDetach notifies the daemon that this client is leaving (disconnect notification).
// The daemon keeps loops running.
func (c *Client) SendDetach(ctx context.Context) error {
	return c.SendMessage(ctx, NewDisconnectEnvelope())
}

// SendDaemonStatus requests daemon status (request envelope). The caller reads
// the response via ReadEvent or the blocking DaemonStatus convenience method.
func (c *Client) SendDaemonStatus(ctx context.Context, requestID ...string) error {
	rid := optRequestID(requestID)
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("daemon_status", map[string]interface{}{}, rid))
}

// SendDaemonShutdown requests daemon shutdown (request envelope).
func (c *Client) SendDaemonShutdown(ctx context.Context, requestID ...string) error {
	rid := optRequestID(requestID)
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("daemon_shutdown", map[string]interface{}{}, rid))
}

// SendConfigGet requests a config section from the daemon (request envelope).
func (c *Client) SendConfigGet(ctx context.Context, section string, requestID ...string) error {
	rid := optRequestID(requestID)
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("config_get", map[string]interface{}{"section": section}, rid))
}

// SendSkillsList requests the skills catalog (request envelope).
func (c *Client) SendSkillsList(ctx context.Context, requestID ...string) error {
	rid := optRequestID(requestID)
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("skills_list", map[string]interface{}{}, rid))
}

// SendModelsList requests the models catalog (request envelope).
func (c *Client) SendModelsList(ctx context.Context, requestID ...string) error {
	rid := optRequestID(requestID)
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("models_list", map[string]interface{}{}, rid))
}

// SendInvokeSkill invokes a skill on the daemon (request envelope).
func (c *Client) SendInvokeSkill(ctx context.Context, skill, args string, requestID ...string) error {
	rid := optRequestID(requestID)
	params := map[string]interface{}{"skill": skill}
	if args != "" {
		params["args"] = args
	}
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("invoke_skill", params, rid))
}

// SendLoopList requests the list of StrangeLoop instances (request envelope).
func (c *Client) SendLoopList(ctx context.Context, filter map[string]interface{}, limit int, requestID ...string) error {
	rid := optRequestID(requestID)
	params := map[string]interface{}{}
	if filter != nil {
		params["filter"] = filter
	}
	if limit > 0 {
		params["limit"] = limit
	}
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("loop_list", params, rid))
}

// SendLoopGet requests details for a specific loop (request envelope).
func (c *Client) SendLoopGet(ctx context.Context, loopID string, verbose bool, requestID ...string) error {
	rid := optRequestID(requestID)
	params := map[string]interface{}{"loop_id": loopID}
	if verbose {
		params["verbose"] = true
	}
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("loop_get", params, rid))
}

// SendLoopTree requests the checkpoint tree for a loop (request envelope).
func (c *Client) SendLoopTree(ctx context.Context, loopID, format string, requestID ...string) error {
	rid := optRequestID(requestID)
	params := map[string]interface{}{"loop_id": loopID}
	if format != "" {
		params["format"] = format
	}
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("loop_tree", params, rid))
}

// SendLoopPrune requests pruning of old branches for a loop (request envelope).
// keep_latest specifies how many recent checkpoints to preserve (minimum 1).
func (c *Client) SendLoopPrune(ctx context.Context, loopID string, keepLatest int, requestID ...string) error {
	rid := optRequestID(requestID)
	params := map[string]interface{}{"loop_id": loopID}
	if keepLatest > 0 {
		params["keep_latest"] = keepLatest
	}
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("loop_prune", params, rid))
}

// SendLoopDelete requests deletion of a loop (request envelope).
func (c *Client) SendLoopDelete(ctx context.Context, loopID string, requestID ...string) error {
	rid := optRequestID(requestID)
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("loop_delete", map[string]interface{}{"loop_id": loopID}, rid))
}

// SendLoopReattach requests reattachment to a loop (request envelope).
func (c *Client) SendLoopReattach(ctx context.Context, loopID string, requestID ...string) error {
	rid := optRequestID(requestID)
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("loop_reattach", map[string]interface{}{"loop_id": loopID}, rid))
}

// SendLoopSubscribe subscribes to loop events (subscribe envelope, method loop_events).
// Pass empty wireTier to omit the field (daemon default).
// Pass empty streamDelivery to omit the field (daemon default is "adaptive").
func (c *Client) SendLoopSubscribe(ctx context.Context, loopID string, wireTier string, streamDelivery string, requestID ...string) error {
	rid := optRequestID(requestID)
	params := map[string]interface{}{"loop_id": loopID}
	if wireTier != "" {
		params["wire_tier"] = wireTier
	}
	if streamDelivery != "" {
		params["stream_delivery"] = streamDelivery
	}
	if rid == "" {
		return c.SendMessage(ctx, NewSubscribeEnvelope("loop_events", params))
	}
	// When an explicit request id is supplied, build the subscribe envelope
	// with that id so the caller can correlate the confirmation.
	env := NewSubscribeEnvelope("loop_events", params)
	env.ID = rid
	return c.SendMessage(ctx, env)
}

// SendLoopDetach detaches from a loop (unsubscribe envelope by subscription id).
func (c *Client) SendLoopDetach(ctx context.Context, loopID string, requestID ...string) error {
	rid := optRequestID(requestID)
	if rid == "" {
		rid = loopID
	}
	return c.SendMessage(ctx, NewUnsubscribeEnvelope(rid))
}

// SendLoopNew creates a new loop (request envelope).
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
	rid := optRequestID(requestID)
	params := map[string]interface{}{}
	if clientWorkspace != "" {
		params["client_workspace"] = clientWorkspace
	}
	if userID != "" {
		params["user_id"] = userID
	}
	if clientWorkspaceID != "" {
		params["client_workspace_id"] = clientWorkspaceID
	}
	if isEphemeral {
		params["is_ephemeral"] = true
	}
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("loop_new", params, rid))
}

// SendLoopInput sends input to a loop as a loop_input notification.
func (c *Client) SendLoopInput(ctx context.Context, loopID string, content string, requestID ...string) error {
	_ = requestID // loop_input is a notification (no id); kept for API stability.
	return c.SendMessage(ctx, NewNotificationEnvelope("loop_input", map[string]interface{}{
		"loop_id": loopID,
		"content": content,
	}))
}

// ---------------------------------------------------------------------------
// Additional loop send methods
// ---------------------------------------------------------------------------

// SendLoopMessages requests persisted conversation/activity rows (request envelope).
func (c *Client) SendLoopMessages(ctx context.Context, loopID string, limit int, offset int, includeEvents bool, requestID ...string) error {
	rid := optRequestID(requestID)
	params := map[string]interface{}{"loop_id": loopID}
	if limit > 0 {
		params["limit"] = limit
	}
	if offset > 0 {
		params["offset"] = offset
	}
	if includeEvents {
		params["include_events"] = true
	}
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("loop_messages", params, rid))
}

// SendLoopStateGet requests LangGraph checkpoint channel values (request envelope).
func (c *Client) SendLoopStateGet(ctx context.Context, loopID string, requestID ...string) error {
	rid := optRequestID(requestID)
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("loop_state_get", map[string]interface{}{"loop_id": loopID}, rid))
}

// SendLoopStateUpdate applies partial checkpoint values (request envelope).
func (c *Client) SendLoopStateUpdate(ctx context.Context, loopID string, values map[string]interface{}, asNode string, requestID ...string) error {
	rid := optRequestID(requestID)
	params := map[string]interface{}{"loop_id": loopID, "values": values}
	if asNode != "" {
		params["as_node"] = asNode
	}
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("loop_state_update", params, rid))
}

// SendLoopCardsFetch requests display card ledger (request envelope).
func (c *Client) SendLoopCardsFetch(ctx context.Context, loopID string, requestID ...string) error {
	rid := optRequestID(requestID)
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("loop_cards_fetch", map[string]interface{}{"loop_id": loopID}, rid))
}

// SendMCPStatus requests MCP server status (request envelope).
func (c *Client) SendMCPStatus(ctx context.Context, requestID ...string) error {
	rid := optRequestID(requestID)
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("mcp_status", map[string]interface{}{}, rid))
}

// SendLoopHistoryFetch requests the loop's replayable history (request envelope).
func (c *Client) SendLoopHistoryFetch(ctx context.Context, loopID string, requestID ...string) error {
	rid := optRequestID(requestID)
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("loop_history_fetch", map[string]interface{}{"loop_id": loopID}, rid))
}

// SendConfigReload requests a config reload on the daemon (request envelope).
func (c *Client) SendConfigReload(ctx context.Context, requestID ...string) error {
	rid := optRequestID(requestID)
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("config_reload", map[string]interface{}{}, rid))
}

// SendAuth submits access_key/secret_key credentials to the daemon (request envelope).
func (c *Client) SendAuth(ctx context.Context, accessKey, secretKey string, requestID ...string) error {
	rid := optRequestID(requestID)
	params := map[string]interface{}{"access_key": accessKey, "secret_key": secretKey}
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("auth", params, rid))
}

// SendAuthRefresh submits a refresh_token to the daemon (request envelope).
func (c *Client) SendAuthRefresh(ctx context.Context, refreshToken string, requestID ...string) error {
	rid := optRequestID(requestID)
	params := map[string]interface{}{"refresh_token": refreshToken}
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("auth_refresh", params, rid))
}

// SendCronAdd creates a scheduled job from natural language (request envelope).
func (c *Client) SendCronAdd(ctx context.Context, text string, priority int, requestID ...string) error {
	rid := optRequestID(requestID)
	params := map[string]interface{}{"text": text}
	if priority > 0 {
		params["priority"] = priority
	}
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("cron_add", params, rid))
}

// SendCronList lists scheduled jobs (request envelope).
func (c *Client) SendCronList(ctx context.Context, status string, requestID ...string) error {
	rid := optRequestID(requestID)
	params := map[string]interface{}{}
	if status != "" {
		params["status"] = status
	}
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("cron_list", params, rid))
}

// SendCronShow shows a specific scheduled job (request envelope).
func (c *Client) SendCronShow(ctx context.Context, jobID string, requestID ...string) error {
	rid := optRequestID(requestID)
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("cron_show", map[string]interface{}{"job_id": jobID}, rid))
}

// SendCronCancel cancels a scheduled job (request envelope).
func (c *Client) SendCronCancel(ctx context.Context, jobID string, requestID ...string) error {
	rid := optRequestID(requestID)
	return c.SendMessage(ctx, NewRequestEnvelopeWithID("cron_cancel", map[string]interface{}{"job_id": jobID}, rid))
}

// optRequestID returns the first non-empty request id from the variadic args,
// or "" ( signalling the caller to generate one) when none is provided.
func optRequestID(requestID []string) string {
	if len(requestID) > 0 && requestID[0] != "" {
		return requestID[0]
	}
	return ""
}
