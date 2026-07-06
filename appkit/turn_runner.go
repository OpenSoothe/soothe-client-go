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

// ErrQueryTimeout is returned when a turn exceeds the configured timeout.
var ErrQueryTimeout = errors.New("appkit: query timeout")

// TurnConfig configures a TurnRunner.
type TurnConfig struct {
	// QueryTimeout is the per-turn deadline. Defaults to 30m if zero.
	QueryTimeout time.Duration
}

// InputOpts carries the optional daemon hints on a loop_input payload. Apps
// build this from their product modes (e.g. triarch's ask/agent/deep-research).
type InputOpts struct {
	IntentHint           string
	PreferredSubagent    string
	ResponseSchema       map[string]interface{}
	ResponseSchemaName   string
	ResponseSchemaStrict *bool
}

// InputMessageForLoop builds a loop_input payload with optional attachments
// (IG-327: each attachment is {mime_type, data(base64)}). Ported from
// triarch's soothe_hints so apps can construct consistent payloads.
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
// It is the app-agnostic successor to triarch's ExecuteQuery (RFC-629 Layer 1).
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
	onComplete func(sessionID, loopID, content, completionEvent string, elapsedMs int64)
	onError    func(sessionID, loopID string, err error)
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
func (r *TurnRunner) WithOnComplete(f func(sessionID, loopID, content, completionEvent string, elapsedMs int64)) *TurnRunner {
	r.onComplete = f
	return r
}

// WithOnError sets an error hook (runs inline on failure).
func (r *TurnRunner) WithOnError(f func(sessionID, loopID string, err error)) *TurnRunner {
	r.onError = f
	return r
}

// Execute runs one query turn. The response is broadcast via the SSE
// broadcaster and persisted via the SessionStore; it is not returned to the
// caller (SSE subscribers receive it). Returns nil on success, an error on
// failure (ErrQueryTimeout, context.Canceled, or a daemon/processing error).
func (r *TurnRunner) Execute(ctx context.Context, sessionID, message, userID, workspaceID string, attachments []map[string]interface{}, opts *InputOpts) error {
	if opts != nil {
		if h := strings.TrimSpace(opts.IntentHint); h != "" {
			if err := soothe.ValidateLoopInputIntentHint(h); err != nil {
				return fmt.Errorf("appkit: %w", err)
			}
		}
	}
	conn, err := r.pool.Acquire(ctx, sessionID, workspaceID, userID)
	if err != nil {
		r.persistFailed(ctx, sessionID, "", err)
		r.broadcastError(sessionID, err)
		if r.onError != nil {
			r.onError(sessionID, "", err)
		}
		return err
	}
	loopID := conn.getLoopID()

	// Per-turn timeout context, cancellable by QueryGate.Cancel.
	timeoutCtx, cancel := context.WithTimeout(ctx, r.cfg.QueryTimeout)
	defer cancel()

	// Build the daemon-cancel sender for this loop; register with the gate.
	sendCancel := func(c context.Context) error {
		return r.sendLoopCancel(c, conn, loopID)
	}
	if err := r.gate.Acquire(sessionID, cancel, sendCancel); err != nil {
		r.pool.Release(sessionID)
		r.persistFailed(ctx, sessionID, loopID, err)
		r.broadcastError(sessionID, err)
		if r.onError != nil {
			r.onError(sessionID, loopID, err)
		}
		return err
	}
	defer r.gate.Release(sessionID)

	// Send loop_input.
	inputMsg := r.buildInput(message, loopID, attachments, opts)
	if err := conn.client.SendMessage(timeoutCtx, inputMsg); err != nil {
		r.persistFailed(ctx, sessionID, loopID, err)
		r.broadcastError(sessionID, err)
		if r.onError != nil {
			r.onError(sessionID, loopID, err)
		}
		return fmt.Errorf("send message: %w", err)
	}

	eventCh := conn.eventCh
	if eventCh == nil {
		err := fmt.Errorf("missing event stream for session %s (loop %s)", sessionID, loopID)
		r.persistFailed(ctx, sessionID, loopID, err)
		r.broadcastError(sessionID, err)
		if r.onError != nil {
			r.onError(sessionID, loopID, err)
		}
		return err
	}

	var assistantContent string
	startedAt := time.Now()

	for {
		select {
		case <-timeoutCtx.Done():
			if timeoutCtx.Err() == context.Canceled {
				log.Printf("[appkit.TurnRunner] query cancelled for %s (loop %s)", sessionID, loopID)
				err := context.Canceled
				r.persistFailed(ctx, sessionID, loopID, err)
				r.broadcastError(sessionID, err)
				if r.onError != nil {
					r.onError(sessionID, loopID, err)
				}
				return err
			}
			// Timeout: tell the daemon to stop, then persist/broadcast.
			if cerr := r.sendLoopCancel(context.Background(), conn, loopID); cerr != nil {
				log.Printf("[appkit.TurnRunner] WARN: daemon cancel on timeout failed for %s loop %s: %v", sessionID, loopID, cerr)
			}
			r.persistFailed(ctx, sessionID, loopID, ErrQueryTimeout)
			r.broadcastError(sessionID, ErrQueryTimeout)
			if r.onError != nil {
				r.onError(sessionID, loopID, ErrQueryTimeout)
			}
			return ErrQueryTimeout

		case msg, ok := <-eventCh:
			if !ok {
				err := fmt.Errorf("event stream closed")
				r.persistFailed(ctx, sessionID, loopID, err)
				r.broadcastError(sessionID, err)
				if r.onError != nil {
					r.onError(sessionID, loopID, err)
				}
				return err
			}

			eventResult := r.classifier.Classify(msg, assistantContent)
			if eventResult.Err != nil && eventResult.Terminal == ChatEventFailedComplete {
				r.persistFailed(ctx, sessionID, loopID, eventResult.Err)
				r.broadcastError(sessionID, eventResult.Err)
				if r.onError != nil {
					r.onError(sessionID, loopID, eventResult.Err)
				}
				return fmt.Errorf("process event: %w", eventResult.Err)
			}

			if step := strings.TrimSpace(eventResult.ThinkingStep); step != "" {
				r.broadcastThinkingStep(sessionID, step)
			}

			if eventResult.Content != "" {
				// Derive the newly-arrived text to stream as a delta. The
				// daemon may send either cumulative content (each event holds
				// the full text so far, prefixed by what we already have) or
				// true streaming chunks (each event holds only the new text).
				var delta string
				if strings.HasPrefix(eventResult.Content, assistantContent) {
					delta = strings.TrimPrefix(eventResult.Content, assistantContent)
					assistantContent = eventResult.Content
				} else {
					delta = eventResult.Content
					assistantContent += eventResult.Content
				}
				if delta != "" {
					r.broadcastDelta(sessionID, delta)
				}
			}

			if final, deliverable := r.classifier.ResolveDeliverableFinalContent(eventResult, assistantContent); deliverable {
				elapsedMs := time.Since(startedAt).Milliseconds()
				r.persistResponse(ctx, sessionID, loopID, final, startedAt, eventResult.CompletionEvent)
				r.broadcastComplete(sessionID, final)
				if r.onComplete != nil {
					r.onComplete(sessionID, loopID, final, eventResult.CompletionEvent, elapsedMs)
				}
				return nil
			}
		}
	}
}

// sendLoopCancel asks the daemon to cooperatively stop the loop runner on a
// detached timeout context. Mirrors triarch's sendLoopCancelCommand.
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

func (r *TurnRunner) persistResponse(ctx context.Context, sessionID, loopID, content string, startedAt time.Time, completionEvent string) {
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
	if err := r.store.AppendMessage(ctx, sessionID, msg); err != nil {
		log.Printf("[appkit.TurnRunner] persist response failed for %s: %v", sessionID, err)
	}
}

func (r *TurnRunner) persistFailed(ctx context.Context, sessionID, loopID string, err error) {
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
	if err := r.store.AppendMessage(ctx, sessionID, msg); err != nil {
		log.Printf("[appkit.TurnRunner] persist failed-query failed for %s: %v", sessionID, err)
	}
}

func (r *TurnRunner) broadcastThinkingStep(sessionID, step string) {
	if r.broadcaster == nil || strings.TrimSpace(step) == "" {
		return
	}
	// Distinct from "delta": a thinking step is a progress line (e.g. "Tool:
	// search"), not assistant content. Bridges that only stream assistant
	// text can ignore "thinking_step"; those that surface progress can map it
	// to their own event vocabulary.
	r.broadcaster.Broadcast(sessionID, SSEEvent{
		Type: "thinking_step",
		Data: strings.TrimSpace(step) + "\n",
	})
}

// broadcastDelta streams a newly-arrived fragment of assistant content. The
// concatenation of all deltas for a turn equals the final "complete" payload
// (barring deliverable-phase replacement). Streaming is best-effort: a nil
// broadcaster or empty delta is a no-op.
func (r *TurnRunner) broadcastDelta(sessionID, delta string) {
	if r.broadcaster == nil || delta == "" {
		return
	}
	r.broadcaster.Broadcast(sessionID, SSEEvent{
		Type: "delta",
		Data: delta,
	})
}

func (r *TurnRunner) broadcastComplete(sessionID, content string) {
	if r.broadcaster != nil {
		r.broadcaster.Broadcast(sessionID, SSEEvent{
			Type: "complete",
			Data: content,
		})
	}
}

func (r *TurnRunner) broadcastError(sessionID string, err error) {
	if r.broadcaster != nil {
		r.broadcaster.Broadcast(sessionID, SSEEvent{
			Type: "query_error",
			Data: err.Error(),
		})
	}
}
