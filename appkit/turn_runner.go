package appkit

import (
	"context"
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
	ResponseSchema       map[string]interface{}
	ResponseSchemaName   string
	ResponseSchemaStrict *bool
}

// InputMessageForLoop builds a loop_input payload. Each attachment map should
// use mime_type plus base64 data under the key "data".
func InputMessageForLoop(text, loopID string, attachments []map[string]interface{}, opts *InputOpts) map[string]interface{} {
	msg := map[string]interface{}{
		"type":    "loop_input",
		"content": text,
	}
	if loopID != "" {
		msg["loop_id"] = loopID
	}
	if len(attachments) > 0 {
		msg["attachments"] = attachments
	}
	if opts != nil {
		if h := strings.TrimSpace(opts.IntentHint); h != "" {
			msg["intent_hint"] = h
		}
		if s := strings.TrimSpace(opts.PreferredSubagent); s != "" {
			msg["preferred_subagent"] = s
		}
		if len(opts.ResponseSchema) > 0 {
			msg["response_schema"] = opts.ResponseSchema
		}
		if n := strings.TrimSpace(opts.ResponseSchemaName); n != "" {
			msg["response_schema_name"] = n
		}
		if opts.ResponseSchemaStrict != nil {
			msg["response_schema_strict"] = *opts.ResponseSchemaStrict
		}
	}
	return msg
}

// TurnRunner executes one query turn end-to-end: acquire a pooled connection,
// enforce single-flight, send loop_input, consume the event stream, classify
// events, resolve the deliverable, persist the reply, and broadcast completion.
type TurnRunner struct {
	pool        *ConnectionPool
	gate        *QueryGate
	classifier  *EventClassifier
	store       SessionStore
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
func NewTurnRunner(pool *ConnectionPool, gate *QueryGate, cl *EventClassifier, store SessionStore, b *SSEBroadcaster, cfg TurnConfig) *TurnRunner {
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
// broadcaster and persisted via the SessionStore; it is not returned to the
// caller (SSE subscribers receive it). Returns nil on success, an error on
// failure (ErrQueryTimeout, ErrIdleTimeout, context.Canceled, or a daemon/processing error).
func (r *TurnRunner) Execute(ctx context.Context, appKey, message, userID, workspaceID string, attachments []map[string]interface{}, opts *InputOpts) error {
	if err := r.validateOpts(opts); err != nil {
		return err
	}
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
	if err := r.validateOpts(opts); err != nil {
		return err
	}
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

func (r *TurnRunner) validateOpts(opts *InputOpts) error {
	if opts == nil {
		return nil
	}
	if h := strings.TrimSpace(opts.IntentHint); h != "" {
		if err := soothe.ValidateLoopInputIntentHint(h); err != nil {
			return fmt.Errorf("appkit: %w", err)
		}
	}
	return nil
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

	inputMsg := r.buildInput(message, loopID, atts, opts)
	if err := conn.client.SendMessage(timeoutCtx, inputMsg); err != nil {
		r.persistFailed(ctx, appKey, loopID, err)
		r.broadcastError(appKey, err)
		if r.onError != nil {
			r.onError(appKey, loopID, err)
		}
		return fmt.Errorf("send message: %w", err)
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

	var assistantContent string
	startedAt := time.Now()
	// Per-turn gate for stream.end / gated idle (shared classifier stays race-free).
	var turnGate *TurnLifecycleGate
	if r.classifier != nil && (r.classifier.cfg.TreatStreamEndAsComplete || r.classifier.cfg.GateTurnEndSignals) {
		turnGate = &TurnLifecycleGate{}
	}

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

			eventResult := r.classifier.ClassifyTurn(msg, assistantContent, turnGate)
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
				return r.completeTurn(ctx, appKey, loopID, final, startedAt, eventResult.CompletionEvent)
			}
		}
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
