package appkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	soothe "github.com/mirasoth/soothe-client-go"
)

// ErrQueryTimeout is returned when a turn exceeds the configured timeout
// and OnQueryTimeout is TimeoutPolicyFail (default).
var ErrQueryTimeout = errors.New("appkit: query timeout")

// ErrIdleTimeout is returned when no events arrive within IdleTimeout
// and OnIdleTimeout is TimeoutPolicyFail (default).
var ErrIdleTimeout = errors.New("appkit: idle timeout")

// TimeoutPolicy selects fail vs soft-complete behaviour for idle, query, and
// stream-close terminals.
type TimeoutPolicy int

const (
	// TimeoutPolicyFail ends the turn with an error (default).
	TimeoutPolicyFail TimeoutPolicy = iota
	// TimeoutPolicySoftComplete persists/broadcasts accumulated content as
	// success when any text was received; otherwise falls back to Fail.
	TimeoutPolicySoftComplete
)

// StreamClosePolicy is an alias of TimeoutPolicy for OnStreamClose.
type StreamClosePolicy = TimeoutPolicy

const (
	StreamCloseFail         = TimeoutPolicyFail
	StreamCloseSoftComplete = TimeoutPolicySoftComplete
)

// TurnConfig configures a TurnRunner.
type TurnConfig struct {
	// QueryTimeout is the per-turn deadline. Defaults to 30m if zero.
	QueryTimeout time.Duration

	// IdleTimeout is the maximum silence between consecutive classified
	// events. Zero disables (default). Each received event resets the timer.
	IdleTimeout time.Duration

	// MinIdleTimeoutWithAttachments, when > 0, raises IdleTimeout for a turn
	// that has attachments if IdleTimeout is positive but below this floor
	// (useful for image_to_text gaps). Zero = no floor.
	MinIdleTimeoutWithAttachments time.Duration

	// OnIdleTimeout selects fail vs soft-complete when the idle watchdog fires.
	// Default TimeoutPolicyFail.
	OnIdleTimeout TimeoutPolicy

	// OnQueryTimeout selects fail vs soft-complete when QueryTimeout fires.
	// Default TimeoutPolicyFail.
	OnQueryTimeout TimeoutPolicy

	// OnStreamClose selects fail vs soft-complete when the event channel closes.
	// Default StreamCloseFail.
	OnStreamClose StreamClosePolicy

	// CompactAttachmentsBeforeSend runs CompactAttachments on the turn's
	// attachment maps before buildInput. Default false.
	CompactAttachmentsBeforeSend bool

	// CompactImageOpts overrides defaults for CompactAttachmentsBeforeSend.
	CompactImageOpts *CompactImageOptions
}

// InputOpts carries optional daemon hints on a loop_input payload (intent,
// subagent preference, structured-output schema, and related fields).
type InputOpts struct {
	IntentHint           string
	PreferredSubagent    string
	IntakeScope          string
	InteractionMode      string
	ResponseSchema       map[string]interface{}
	ResponseSchemaName   string
	ResponseSchemaStrict *bool
}

// InputMessageForLoop builds a protocol-1 loop_input notification envelope.
// Each attachment map should use mime_type plus base64 data under the key "data".
// The returned map is suitable for Client.SendMessage (same shape as SendInput).
func InputMessageForLoop(text, loopID string, attachments []map[string]interface{}, opts *InputOpts) map[string]interface{} {
	params := map[string]interface{}{
		"content": text,
	}
	if loopID != "" {
		params["loop_id"] = loopID
	}
	if len(attachments) > 0 {
		params["attachments"] = attachments
	}
	if opts != nil {
		if h := strings.TrimSpace(opts.IntentHint); h != "" {
			params["intent_hint"] = h
		}
		if s := strings.TrimSpace(opts.PreferredSubagent); s != "" {
			params["preferred_subagent"] = s
		}
		if s := strings.TrimSpace(opts.IntakeScope); s != "" {
			params["intake_scope"] = s
		}
		if m := strings.TrimSpace(opts.InteractionMode); m != "" {
			params["interaction_mode"] = m
		}
		if len(opts.ResponseSchema) > 0 {
			params["response_schema"] = opts.ResponseSchema
		}
		if n := strings.TrimSpace(opts.ResponseSchemaName); n != "" {
			params["response_schema_name"] = n
		}
		if opts.ResponseSchemaStrict != nil {
			params["response_schema_strict"] = *opts.ResponseSchemaStrict
		}
	}
	env := soothe.NewNotificationEnvelope("loop_input", params)
	raw, err := json.Marshal(env)
	if err != nil {
		return map[string]interface{}{
			"proto":  soothe.ProtoVersion,
			"type":   "notification",
			"method": "loop_input",
			"params": params,
		}
	}
	var msg map[string]interface{}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return map[string]interface{}{
			"proto":  soothe.ProtoVersion,
			"type":   "notification",
			"method": "loop_input",
			"params": params,
		}
	}
	return msg
}

// TurnRunner executes one query turn end-to-end: acquire a pooled connection,
// enforce single-flight, send loop_input, consume the event stream, classify
// content, persist/broadcast.
//
// Turn end is owned by TurnBoundary (DaemonSession.IterTurnChunks contract:
// stream.end / gated idle / stopped). EventClassifier selects user-visible
// text and may early-complete on deliverable phases for UX; it is not the
// sole turn terminator.
type TurnRunner struct {
	pool        *ConnectionPool
	gate        *QueryGate
	classifier  *EventClassifier
	store       LoopSessionStore
	broadcaster *SSEBroadcaster
	cfg         TurnConfig

	// buildInput lets an app customize the loop_input payload (e.g. inject a
	// generated request_id or app-specific fields). Defaults to InputMessageForLoop.
	buildInput func(text, loopID string, attachments []map[string]interface{}, opts *InputOpts) map[string]interface{}

	// onComplete / onError hooks let an app react to turn outcomes without
	// reimplementing the loop. They run inline; keep them cheap.
	onComplete func(appKey, loopID, content, completionEvent string, elapsedMs int64)
	onError    func(appKey, loopID string, err error)

	// errorData formats the SSE query_error Data field. Default: err.Error().
	errorData func(error) interface{}
}

// NewTurnRunner constructs a TurnRunner. pool, gate, classifier, and store are
// required; broadcaster may be nil.
func NewTurnRunner(pool *ConnectionPool, gate *QueryGate, cl *EventClassifier, store LoopSessionStore, b *SSEBroadcaster, cfg TurnConfig) *TurnRunner {
	tr := &TurnRunner{
		pool:        pool,
		gate:        gate,
		classifier:  cl,
		store:       store,
		broadcaster: b,
		cfg:         cfg,
		buildInput:  InputMessageForLoop,
	}
	if cfg.QueryTimeout <= 0 {
		tr.cfg.QueryTimeout = 30 * time.Minute
	}
	return tr
}

// WithInputBuilder overrides the loop_input payload builder.
func (r *TurnRunner) WithInputBuilder(f func(text, loopID string, attachments []map[string]interface{}, opts *InputOpts) map[string]interface{}) *TurnRunner {
	if f != nil {
		r.buildInput = f
	}
	return r
}

// WithOnComplete sets a completion hook (runs inline on success).
func (r *TurnRunner) WithOnComplete(f func(appKey, loopID, content, completionEvent string, elapsedMs int64)) *TurnRunner {
	r.onComplete = f
	return r
}

// WithOnError sets an error hook (runs inline on failure).
func (r *TurnRunner) WithOnError(f func(appKey, loopID string, err error)) *TurnRunner {
	r.onError = f
	return r
}

// WithErrorData sets a formatter for SSE query_error payloads (e.g. structured JSON).
func (r *TurnRunner) WithErrorData(f func(error) interface{}) *TurnRunner {
	if f != nil {
		r.errorData = f
	}
	return r
}

func idleTimeoutForTurn(cfg TurnConfig, hasAttachments bool) time.Duration {
	idle := cfg.IdleTimeout
	if idle <= 0 {
		return 0
	}
	if hasAttachments && cfg.MinIdleTimeoutWithAttachments > 0 && idle < cfg.MinIdleTimeoutWithAttachments {
		return cfg.MinIdleTimeoutWithAttachments
	}
	return idle
}

// Execute runs one query turn. The response is broadcast via the SSE
// broadcaster and persisted via the LoopSessionStore; it is not returned to the
// caller (SSE subscribers receive it). Returns nil on success, an error on
// failure (ErrQueryTimeout, ErrIdleTimeout, context.Canceled, or a daemon/processing error).
func (r *TurnRunner) Execute(ctx context.Context, appKey, message, userID, workspaceID string, attachments []map[string]interface{}, opts *InputOpts) error {
	conn, err := r.pool.Acquire(ctx, appKey, workspaceID, userID)
	if err != nil {
		r.persistFailed(ctx, appKey, "", err)
		r.broadcastError(appKey, err)
		if r.onError != nil {
			r.onError(appKey, "", err)
		}
		return err
	}
	loopID := conn.getLoopID()

	// Per-turn timeout context, cancellable by QueryGate.Cancel.
	timeoutCtx, cancel := context.WithTimeout(ctx, r.cfg.QueryTimeout)
	defer cancel()

	sendCancel := func(c context.Context) error {
		return r.sendLoopCancel(c, conn, loopID)
	}
	if err := r.gate.Acquire(appKey, cancel, sendCancel); err != nil {
		r.pool.Release(appKey)
		r.persistFailed(ctx, appKey, loopID, err)
		r.broadcastError(appKey, err)
		if r.onError != nil {
			r.onError(appKey, loopID, err)
		}
		return err
	}
	defer r.gate.Release(appKey)

	return r.runTurn(ctx, timeoutCtx, conn, appKey, loopID, message, attachments, opts)
}

// ExecuteReserved runs a turn when the caller already reserved the gate via
// QueryGate.Acquire (e.g. HTTP handler returns 409 before spawning a goroutine).
// It replaces the cancel with a timeout-derived cancel, registers sendCancel,
// and Releases the gate on exit.
func (r *TurnRunner) ExecuteReserved(ctx context.Context, appKey, message, userID, workspaceID string, attachments []map[string]interface{}, opts *InputOpts) error {
	if r.gate == nil || !r.gate.IsActive(appKey) {
		return fmt.Errorf("appkit: ExecuteReserved requires an active QueryGate reservation for %s", appKey)
	}
	conn, err := r.pool.Acquire(ctx, appKey, workspaceID, userID)
	if err != nil {
		r.persistFailed(ctx, appKey, "", err)
		r.broadcastError(appKey, err)
		if r.onError != nil {
			r.onError(appKey, "", err)
		}
		r.gate.Release(appKey)
		return err
	}
	loopID := conn.getLoopID()

	timeoutCtx, cancel := context.WithTimeout(ctx, r.cfg.QueryTimeout)
	defer cancel()
	r.gate.ReplaceCancel(appKey, cancel)
	r.gate.SetSendCancel(appKey, func(c context.Context) error {
		return r.sendLoopCancel(c, conn, loopID)
	})
	defer r.gate.Release(appKey)

	return r.runTurn(ctx, timeoutCtx, conn, appKey, loopID, message, attachments, opts)
}

func (r *TurnRunner) runTurn(
	ctx context.Context,
	timeoutCtx context.Context,
	conn *pooledConn,
	appKey, loopID, message string,
	attachments []map[string]interface{},
	opts *InputOpts,
) error {
	atts := attachments
	if r.cfg.CompactAttachmentsBeforeSend && len(atts) > 0 {
		atts = CompactAttachments(atts, r.cfg.CompactImageOpts)
	}

	eventCh := conn.eventCh
	if eventCh == nil {
		err := fmt.Errorf("missing event stream for app key %s (loop %s)", appKey, loopID)
		r.persistFailed(ctx, appKey, loopID, err)
		r.broadcastError(appKey, err)
		if r.onError != nil {
			r.onError(appKey, loopID, err)
		}
		return err
	}
	// Drop leftovers from a prior turn (or early-complete) before send. A stale
	// status=running would otherwise arm this turn, then a buffered idle would
	// end it with "no assistant content". Settle briefly so the pool forwarder
	// can push in-flight frames into eventCh. Tests release scripted events
	// from SendMessage so this drain does not wipe the fixture stream.
	drainEventCh(eventCh, 5*time.Millisecond)
	defer drainEventCh(eventCh, 0)

	inputMsg := r.buildInput(message, loopID, atts, opts)
	if err := conn.client.SendMessage(timeoutCtx, inputMsg); err != nil {
		r.persistFailed(ctx, appKey, loopID, err)
		r.broadcastError(appKey, err)
		if r.onError != nil {
			r.onError(appKey, loopID, err)
		}
		return fmt.Errorf("send message: %w", err)
	}

	var assistantContent string
	startedAt := time.Now()
	// DaemonSession turn-end contract (always on; not classifier opt-in).
	boundary := &TurnBoundary{}
	// Ignore leftover turn-end signals that race in after the pre-send drain
	// until this turn observes running or assistant content after SendMessage.
	armed := false

	idleForTurn := idleTimeoutForTurn(r.cfg, len(attachments) > 0)
	var idleTimer *time.Timer
	var idleCh <-chan time.Time
	if idleForTurn > 0 {
		idleTimer = time.NewTimer(idleForTurn)
		defer idleTimer.Stop()
		idleCh = idleTimer.C
	}

	resetIdle := func() {
		if idleTimer == nil {
			return
		}
		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		idleTimer.Reset(idleForTurn)
	}

	for {
		select {
		case <-timeoutCtx.Done():
			if timeoutCtx.Err() == context.Canceled {
				log.Printf("[appkit.TurnRunner] query cancelled for %s (loop %s)", appKey, loopID)
				err := context.Canceled
				r.persistFailed(ctx, appKey, loopID, err)
				r.broadcastError(appKey, err)
				if r.onError != nil {
					r.onError(appKey, loopID, err)
				}
				return err
			}
			if cerr := r.sendLoopCancel(context.Background(), conn, loopID); cerr != nil {
				log.Printf("[appkit.TurnRunner] WARN: daemon cancel on timeout failed for %s loop %s: %v", appKey, loopID, cerr)
			}
			return r.finishTimeout(ctx, appKey, loopID, assistantContent, startedAt, ErrQueryTimeout, "query_timeout", r.cfg.OnQueryTimeout)

		case <-idleCh:
			if cerr := r.sendLoopCancel(context.Background(), conn, loopID); cerr != nil {
				log.Printf("[appkit.TurnRunner] WARN: daemon cancel on idle timeout failed for %s loop %s: %v", appKey, loopID, cerr)
			}
			return r.finishTimeout(ctx, appKey, loopID, assistantContent, startedAt, ErrIdleTimeout, "idle_timeout", r.cfg.OnIdleTimeout)

		case msg, ok := <-eventCh:
			if !ok {
				if r.cfg.OnStreamClose == StreamCloseSoftComplete && strings.TrimSpace(assistantContent) != "" {
					return r.completeTurn(ctx, appKey, loopID, assistantContent, startedAt, "stream_closed")
				}
				err := fmt.Errorf("event stream closed")
				r.persistFailed(ctx, appKey, loopID, err)
				r.broadcastError(appKey, err)
				if r.onError != nil {
					r.onError(appKey, loopID, err)
				}
				return err
			}
			resetIdle()

			if !armed {
				if isStaleTurnEndEvent(msg) {
					// Discard buffered end markers from a prior turn.
					continue
				}
				if isStatusRunningEvent(msg) {
					// Track running for TurnBoundary gating but do not arm yet —
					// a leftover running+idle pair must not terminate this turn.
					boundary.Feed(msg)
					continue
				}
				// First non-end, non-running frame after send (content or progress).
				armed = true
			}

			ended, endReason := boundary.Feed(msg)
			if ended && !armed {
				boundary = &TurnBoundary{}
				continue
			}

			// Classify for content / phase early-complete only. Turn end is boundary.
			eventResult := r.classifier.Classify(msg, assistantContent)
			if eventResult.Err != nil && eventResult.Terminal == ChatEventFailedComplete {
				r.persistFailed(ctx, appKey, loopID, eventResult.Err)
				r.broadcastError(appKey, eventResult.Err)
				if r.onError != nil {
					r.onError(appKey, loopID, eventResult.Err)
				}
				return fmt.Errorf("process event: %w", eventResult.Err)
			}

			if step := strings.TrimSpace(eventResult.ThinkingStep); step != "" {
				r.broadcastThinkingStep(appKey, step)
			}

			if eventResult.Content != "" {
				var delta string
				if strings.HasPrefix(eventResult.Content, assistantContent) {
					delta = strings.TrimPrefix(eventResult.Content, assistantContent)
					assistantContent = eventResult.Content
				} else {
					delta = eventResult.Content
					assistantContent += eventResult.Content
				}
				if delta != "" {
					r.broadcastDelta(appKey, delta)
				}
			}

			if final, deliverable := r.classifier.ResolveDeliverableFinalContent(eventResult, assistantContent); deliverable {
				// Phase early-complete (chitchat / goal_completion / intent-hint).
				// Ignore classifier terminals that duplicate TurnBoundary signals.
				if !IsDaemonTurnEndEvent(eventResult.CompletionEvent) {
					return r.completeTurn(ctx, appKey, loopID, final, startedAt, eventResult.CompletionEvent)
				}
			}

			if ended {
				if r.classifier.IsSubstantiveAssistantReply(assistantContent) {
					return r.completeTurn(ctx, appKey, loopID, strings.TrimSpace(assistantContent), startedAt, endReason)
				}
				// Daemon turn closed with no usable reply — fail rather than hang
				// until QueryTimeout (大荔-class stuck thinking).
				err := fmt.Errorf("turn ended (%s) with no assistant content", endReason)
				r.persistFailed(ctx, appKey, loopID, err)
				r.broadcastError(appKey, err)
				if r.onError != nil {
					r.onError(appKey, loopID, err)
				}
				return err
			}
		}
	}
}

// drainEventCh drains a pooled event channel. When settle > 0, keeps reading
// until the channel has been quiet for settle (covers the pool forwarder race
// where leftovers are still in-flight on Acquire).
func drainEventCh(ch <-chan interface{}, settle time.Duration) {
	if ch == nil {
		return
	}
	if settle <= 0 {
		for {
			select {
			case _, ok := <-ch:
				if !ok {
					return
				}
			default:
				return
			}
		}
	}
	deadline := time.Now().Add(settle)
	for {
		timeout := time.Until(deadline)
		if timeout < 0 {
			timeout = 0
		}
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
			// Activity extends the quiet window so a burst of leftovers is fully
			// consumed before SendMessage.
			deadline = time.Now().Add(settle)
		case <-time.After(timeout):
			for {
				select {
				case _, ok := <-ch:
					if !ok {
						return
					}
				default:
					return
				}
			}
		}
	}
}

// isStatusRunningEvent reports a daemon status=running frame.
func isStatusRunningEvent(msg interface{}) bool {
	m, ok := msg.(soothe.StatusResponse)
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(m.State), "running")
}

// isStaleTurnEndEvent reports buffered end markers from a previous turn that
// must not terminate the current turn before it has armed.
func isStaleTurnEndEvent(msg interface{}) bool {
	switch m := msg.(type) {
	case soothe.StatusResponse:
		state := strings.TrimSpace(m.State)
		return strings.EqualFold(state, "idle") || strings.EqualFold(state, "stopped")
	case soothe.EventMessage:
		return m.Mode == "custom" && soothe.IsTurnEndCustomData(m.Data)
	default:
		return false
	}
}

func (r *TurnRunner) finishTimeout(ctx context.Context, appKey, loopID, content string, startedAt time.Time, failErr error, completionEvent string, policy TimeoutPolicy) error {
	if policy == TimeoutPolicySoftComplete && strings.TrimSpace(content) != "" {
		return r.completeTurn(ctx, appKey, loopID, content, startedAt, completionEvent)
	}
	r.persistFailed(ctx, appKey, loopID, failErr)
	r.broadcastError(appKey, failErr)
	if r.onError != nil {
		r.onError(appKey, loopID, failErr)
	}
	return failErr
}

func (r *TurnRunner) completeTurn(ctx context.Context, appKey, loopID, final string, startedAt time.Time, completionEvent string) error {
	elapsedMs := time.Since(startedAt).Milliseconds()
	r.persistResponse(ctx, appKey, loopID, final, startedAt, completionEvent)
	r.broadcastComplete(appKey, final)
	if r.onComplete != nil {
		r.onComplete(appKey, loopID, final, completionEvent, elapsedMs)
	}
	return nil
}

// sendLoopCancel asks the daemon to cooperatively stop the loop runner on a
// detached timeout context (command_request with command "cancel").
func (r *TurnRunner) sendLoopCancel(ctx context.Context, conn *pooledConn, loopID string) error {
	loopID = strings.TrimSpace(loopID)
	if conn == nil || conn.client == nil || loopID == "" {
		return nil
	}
	cancelCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	msg := map[string]interface{}{
		"type":    "command_request",
		"command": "cancel",
		"loop_id": loopID,
	}
	if err := conn.client.SendMessage(cancelCtx, msg); err != nil {
		return fmt.Errorf("send cancel command_request: %w", err)
	}
	return nil
}

func (r *TurnRunner) persistResponse(ctx context.Context, appKey, loopID, content string, startedAt time.Time, completionEvent string) {
	if r.store == nil {
		return
	}
	msg := SessionMessage{
		Role:    "assistant",
		Content: content,
		Metadata: map[string]interface{}{
			"started_at":       startedAt,
			"completed_at":     time.Now(),
			"duration_ms":      time.Since(startedAt).Milliseconds(),
			"status":           "completed",
			"completion_event": completionEvent,
			"deliverable":      true,
		},
	}
	if err := r.store.AppendMessage(ctx, appKey, msg); err != nil {
		log.Printf("[appkit.TurnRunner] persist response failed for %s: %v", appKey, err)
	}
}

func (r *TurnRunner) persistFailed(ctx context.Context, appKey, loopID string, err error) {
	if r.store == nil {
		return
	}
	msg := SessionMessage{
		Role:    "error",
		Content: err.Error(),
		Metadata: map[string]interface{}{
			"status":        "failed",
			"error_message": err.Error(),
		},
	}
	if err := r.store.AppendMessage(ctx, appKey, msg); err != nil {
		log.Printf("[appkit.TurnRunner] persist failed-query failed for %s: %v", appKey, err)
	}
}

func (r *TurnRunner) broadcastThinkingStep(appKey, step string) {
	if r.broadcaster == nil || strings.TrimSpace(step) == "" {
		return
	}
	r.broadcaster.Broadcast(appKey, SSEEvent{
		Type: "thinking_step",
		Data: strings.TrimSpace(step) + "\n",
	})
}

// broadcastDelta streams a newly-arrived fragment of assistant content.
func (r *TurnRunner) broadcastDelta(appKey, delta string) {
	if r.broadcaster == nil || delta == "" {
		return
	}
	r.broadcaster.Broadcast(appKey, SSEEvent{
		Type: "delta",
		Data: delta,
	})
}

func (r *TurnRunner) broadcastComplete(appKey, content string) {
	if r.broadcaster != nil {
		r.broadcaster.Broadcast(appKey, SSEEvent{
			Type: "complete",
			Data: content,
		})
	}
}

func (r *TurnRunner) broadcastError(appKey string, err error) {
	if r.broadcaster == nil {
		return
	}
	var data interface{} = err.Error()
	if r.errorData != nil {
		data = r.errorData(err)
	}
	r.broadcaster.Broadcast(appKey, SSEEvent{
		Type: "query_error",
		Data: data,
	})
}
