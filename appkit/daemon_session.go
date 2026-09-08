package appkit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	soothe "github.com/mirasoth/soothe-client-go"
)

// DefaultPostIdleDrain is how long IterTurnChunks drains after idle/stream end.
const DefaultPostIdleDrain = 500 * time.Millisecond

// TurnChunk is one streamed turn frame: namespace, mode, data.
type TurnChunk struct {
	Namespace []interface{}
	Mode      string
	Data      interface{}
}

// EarlyDropFn filters non-actionable stream chunks before yield.
type EarlyDropFn func(namespace []interface{}, mode string, data interface{}) bool

// DaemonSession is a dual-socket loop session.
// Stream socket carries turns/events; RPC sidecar handles metadata RPCs.
type DaemonSession struct {
	wsURL                string
	workspace            string
	streamDelivery       string
	postIdleDrain        time.Duration
	cfg                  *soothe.Config
	client               *soothe.Client
	rpcClient            *soothe.Client
	loopID               string
	rpcConnected         bool
	closed               bool
	earlyDropFn          EarlyDropFn
	mu                   sync.Mutex
	readMu               sync.Mutex
	rpcMu                sync.Mutex
	TurnEventStats       *TurnEventStats
	LastTurnEndState     string
	LastTurnCancelSeen   bool
	LastTurnErrorMessage string
}

// DaemonSessionOptions configures DaemonSession construction.
type DaemonSessionOptions struct {
	Workspace      string
	StreamDelivery string
	PostIdleDrain  time.Duration
	Config         *soothe.Config
	EarlyDropFn    EarlyDropFn
}

// NewDaemonSession creates a dual-socket session for wsURL.
func NewDaemonSession(wsURL string, opts *DaemonSessionOptions) *DaemonSession {
	if opts == nil {
		opts = &DaemonSessionOptions{}
	}
	cfg := opts.Config
	if cfg == nil {
		cfg = soothe.DefaultConfig()
	}
	delivery := opts.StreamDelivery
	if delivery == "" {
		delivery = "adaptive"
	}
	drain := opts.PostIdleDrain
	if drain <= 0 {
		drain = DefaultPostIdleDrain
	}
	dropFn := opts.EarlyDropFn
	if dropFn == nil {
		dropFn = ShouldDropStreamChunkEarly
	}
	return &DaemonSession{
		wsURL:          wsURL,
		workspace:      opts.Workspace,
		streamDelivery: delivery,
		postIdleDrain:  drain,
		cfg:            cfg,
		client:         soothe.NewClient(wsURL, cfg),
		rpcClient:      soothe.NewClient(wsURL, cfg),
		earlyDropFn:    dropFn,
		TurnEventStats: NewTurnEventStats(),
	}
}

// StreamClient returns the subscribed stream WebSocket.
func (s *DaemonSession) StreamClient() *soothe.Client { return s.client }

// RPCClient returns the RPC sidecar WebSocket.
func (s *DaemonSession) RPCClient() *soothe.Client { return s.rpcClient }

// LoopID returns the active StrangeLoop id.
func (s *DaemonSession) LoopID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loopID
}

// Connect opens the stream socket and bootstraps (or resumes) a loop.
func (s *DaemonSession) Connect(ctx context.Context, resumeLoopID string) (map[string]interface{}, error) {
	if err := soothe.ConnectWithRetries(ctx, s.client, 40, 250*time.Millisecond); err != nil {
		return nil, err
	}
	return s.bootstrapLoop(ctx, resumeLoopID)
}

func (s *DaemonSession) bootstrapLoop(ctx context.Context, resumeLoopID string) (map[string]interface{}, error) {
	var sessionOpts *soothe.LoopSessionOptions
	if s.workspace != "" {
		sessionOpts = &soothe.LoopSessionOptions{ClientWorkspace: s.workspace}
	}
	loopID, err := soothe.BootstrapLoopSession(ctx, s.client, resumeLoopID, s.workspace, s.cfg, sessionOpts)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.loopID = loopID
	s.mu.Unlock()
	return map[string]interface{}{
		"type":    "status",
		"loop_id": loopID,
		"state":   "ready",
	}, nil
}

// NewLoop starts a fresh StrangeLoop conversation.
func (s *DaemonSession) NewLoop(ctx context.Context) (map[string]interface{}, error) {
	return s.bootstrapLoop(ctx, "")
}

// SwitchLoop re-subscribes to an existing loop.
func (s *DaemonSession) SwitchLoop(ctx context.Context, loopID string) (map[string]interface{}, error) {
	return s.bootstrapLoop(ctx, loopID)
}

// EnsureConnected reconnects and re-subscribes when the stream socket died.
func (s *DaemonSession) EnsureConnected(ctx context.Context) error {
	if s.client.IsConnectionAlive() {
		return nil
	}
	resume := s.LoopID()
	s.rpcMu.Lock()
	if s.rpcConnected {
		_ = s.rpcClient.Close()
		s.rpcConnected = false
	}
	s.rpcMu.Unlock()

	if err := s.client.Reconnect(ctx); err != nil {
		_ = s.client.Close()
		if err2 := soothe.ConnectWithRetries(ctx, s.client, 40, 250*time.Millisecond); err2 != nil {
			return err2
		}
	}
	if resume != "" {
		if err := s.client.ReattachAndProbe(ctx, resume); err != nil {
			var stale *soothe.StaleLoopError
			if errors.As(err, &stale) {
				resume = ""
			} else {
				return err
			}
		} else {
			s.mu.Lock()
			s.loopID = resume
			s.mu.Unlock()
			return nil
		}
	}
	_, err := s.bootstrapLoop(ctx, resume)
	return err
}

// Close tears down both sockets.
func (s *DaemonSession) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	_ = s.client.Close()
	_ = s.rpcClient.Close()
	s.rpcMu.Lock()
	s.rpcConnected = false
	s.rpcMu.Unlock()
	return nil
}

// Detach sends a disconnect notification (loops keep running server-side).
func (s *DaemonSession) Detach(ctx context.Context) error {
	if !s.client.IsConnected() {
		return nil
	}
	return s.client.Notify(ctx, "disconnect", map[string]interface{}{})
}

// SendTurnOptions configures SendTurn.
type SendTurnOptions struct {
	PreferredSubagent   string
	Model               string
	ModelParams         map[string]interface{}
	Attachments         []map[string]interface{}
	ClarificationMode   string
	InteractionMode     string
	ClarificationAnswer bool
	IntentHint          string
}

// SendTurn sends user input on the active loop.
func (s *DaemonSession) SendTurn(ctx context.Context, text string, opts *SendTurnOptions) error {
	loopID := s.LoopID()
	if loopID == "" {
		return fmt.Errorf("no active loop session")
	}
	var inputOpts []soothe.InputOption
	inputOpts = append(inputOpts, soothe.WithLoopID(loopID))
	if opts != nil {
		if opts.PreferredSubagent != "" {
			inputOpts = append(inputOpts, soothe.WithSubagent(opts.PreferredSubagent))
		}
		if opts.Model != "" {
			inputOpts = append(inputOpts, soothe.WithModel(opts.Model))
		}
		if opts.ModelParams != nil {
			inputOpts = append(inputOpts, soothe.WithModelParams(opts.ModelParams))
		}
		if opts.Attachments != nil {
			inputOpts = append(inputOpts, soothe.WithAttachments(opts.Attachments))
		}
		if opts.ClarificationMode != "" {
			inputOpts = append(inputOpts, soothe.WithClarificationMode(opts.ClarificationMode))
		}
		if opts.InteractionMode != "" {
			inputOpts = append(inputOpts, soothe.WithInteractionMode(opts.InteractionMode))
		}
		if opts.ClarificationAnswer {
			inputOpts = append(inputOpts, soothe.WithClarificationAnswer())
		}
		if opts.IntentHint != "" {
			inputOpts = append(inputOpts, soothe.WithIntentHint(opts.IntentHint))
		}
	}
	return s.client.SendInput(ctx, text, inputOpts...)
}

// InvokeSkill resolves a skill on the stream socket and waits for the echo.
func (s *DaemonSession) InvokeSkill(ctx context.Context, skill, args, interactionMode string) (map[string]interface{}, error) {
	s.readMu.Lock()
	defer s.readMu.Unlock()
	return s.client.InvokeSkill(ctx, skill, args, interactionMode, 120*time.Second)
}

// CancelActiveTurn requests remote cancel via slash_command.
func (s *DaemonSession) CancelActiveTurn(ctx context.Context) error {
	return s.client.Notify(ctx, "slash_command", map[string]interface{}{"cmd": "/cancel"})
}

// ListLoops fetches loop list via the RPC sidecar.
func (s *DaemonSession) ListLoops(ctx context.Context, limit int) (map[string]interface{}, error) {
	s.rpcMu.Lock()
	defer s.rpcMu.Unlock()
	if err := s.ensureRPCConnectedLocked(ctx); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	return s.rpcClient.LoopList(ctx, nil, limit, 15*time.Second)
}

func (s *DaemonSession) ensureRPCConnectedLocked(ctx context.Context) error {
	if s.rpcConnected && s.rpcClient.IsConnected() {
		return nil
	}
	if err := soothe.ConnectWithRetries(ctx, s.rpcClient, 5, 250*time.Millisecond); err != nil {
		return err
	}
	s.rpcConnected = true
	return nil
}

// FetchLoopHistory loads replayable history via the RPC sidecar.
func (s *DaemonSession) FetchLoopHistory(ctx context.Context, loopID string) (map[string]interface{}, error) {
	s.rpcMu.Lock()
	defer s.rpcMu.Unlock()
	if err := s.ensureRPCConnectedLocked(ctx); err != nil {
		return nil, err
	}
	return soothe.FetchLoopHistory(ctx, s.rpcClient, loopID, 30*time.Second)
}

// SetClarificationMode hot-swaps the agent mode on the running goal.
// Returns applied=true when the swap landed on a live goal, false otherwise.
func (s *DaemonSession) SetClarificationMode(ctx context.Context, mode, interactionMode string) (bool, error) {
	loopID := s.LoopID()
	if loopID == "" {
		return false, nil
	}
	s.rpcMu.Lock()
	defer s.rpcMu.Unlock()
	if err := s.ensureRPCConnectedLocked(ctx); err != nil {
		return false, err
	}
	payload := map[string]interface{}{
		"type":    "loop_set_clarification_mode",
		"loop_id": loopID,
		"mode":    mode,
	}
	if interactionMode != "" {
		payload["interaction_mode"] = interactionMode
	}
	result, err := s.rpcClient.RequestResponse(ctx, payload, "loop_set_clarification_mode_response", 5*time.Second)
	if err != nil {
		return false, err
	}
	applied, _ := result["applied"].(bool)
	return applied, nil
}

// IterTurnChunks streams turn chunks until idle/stopped/stream.end.
// maxWait <= 0 means no absolute deadline.
func (s *DaemonSession) IterTurnChunks(ctx context.Context, maxWait time.Duration) (<-chan TurnChunk, <-chan error) {
	out := make(chan TurnChunk, 32)
	errCh := make(chan error, 1)
	go func() {
		defer close(out)
		defer close(errCh)
		s.readMu.Lock()
		defer s.readMu.Unlock()
		if err := s.iterTurnChunksLocked(ctx, maxWait, out); err != nil {
			errCh <- err
		}
	}()
	return out, errCh
}

func (s *DaemonSession) iterTurnChunksLocked(ctx context.Context, maxWait time.Duration, out chan<- TurnChunk) error {
	s.TurnEventStats = NewTurnEventStats()
	s.LastTurnEndState = ""
	s.LastTurnCancelSeen = false
	s.LastTurnErrorMessage = ""

	// Capture the inbound-drop counter at turn start so the delta can be
	// attributed to this turn (mirrors Python's inbound_dropped_baseline).
	inboundBaseline := s.client.InboundDropped()
	defer func() {
		if s.TurnEventStats != nil {
			s.TurnEventStats.InboundDropped = max(0, s.client.InboundDropped()-inboundBaseline)
		}
	}()

	queryStarted := false
	expectedLoopID := s.LoopID()
	expectedTurnID := ""
	turnProgressSeen := false
	var absoluteDeadline time.Time
	if maxWait > 0 {
		absoluteDeadline = time.Now().Add(maxWait)
	}

	s.client.PeelStalePendingControlEvents()

	for {
		if err := ctx.Err(); err != nil {
			s.LastTurnErrorMessage = err.Error()
			return err
		}
		if !absoluteDeadline.IsZero() && time.Now().After(absoluteDeadline) {
			err := fmt.Errorf("turn timed out after %v (loop=%s)", maxWait, expectedLoopID)
			s.LastTurnErrorMessage = err.Error()
			return err
		}

		ev, err := s.client.ReadEvent()
		if err != nil {
			s.LastTurnErrorMessage = err.Error()
			return err
		}
		if ev == nil {
			if queryStarted && !s.client.IsConnectionAlive() {
				s.LastTurnEndState = "connection_lost"
				err := fmt.Errorf("daemon connection lost")
				s.LastTurnErrorMessage = err.Error()
				return err
			}
			return nil
		}

		frame := ev
		eventType := asString(frame["type"])
		if eventType == "next" {
			frame = UnwrapNext(frame)
			eventType = asString(frame["type"])
		}

		eventLoopID := asString(frame["loop_id"])
		if expectedLoopID != "" && eventLoopID != "" && eventLoopID != expectedLoopID {
			continue
		}

		evTurnID := soothe.FrameTurnID(frame)
		statusState := ""
		if eventType == "status" {
			statusState = asString(frame["state"])
		}
		isRunningStatus := statusState == "running"
		isTerminalStatus := statusState == "idle" || statusState == "stopped"
		if expectedTurnID != "" && (eventType == "event" || eventType == "status") && !isRunningStatus {
			if isTerminalStatus {
				if evTurnID != "" && !soothe.TurnIDsMatch(expectedTurnID, evTurnID) {
					continue
				}
			} else if !soothe.TurnIDsMatch(expectedTurnID, evTurnID) {
				continue
			}
		}

		if eventType == "error" {
			msg := asString(frame["message"])
			if errObj, ok := frame["error"].(map[string]interface{}); ok {
				if m := asString(errObj["message"]); m != "" {
					msg = m
				}
			}
			if msg == "" {
				msg = "daemon error"
			}
			err := fmt.Errorf("%s", msg)
			s.LastTurnErrorMessage = err.Error()
			return err
		}

		if eventType == "status" {
			if lid := asString(frame["loop_id"]); lid != "" {
				s.mu.Lock()
				s.loopID = lid
				s.mu.Unlock()
				expectedLoopID = lid
			}
			switch {
			case statusState == "running":
				queryStarted = true
				if statusTurn := soothe.FrameTurnID(frame); statusTurn != "" {
					newGen := soothe.ParseTurnGeneration(statusTurn)
					oldGen := soothe.ParseTurnGeneration(expectedTurnID)
					if expectedTurnID == "" || (newGen > 0 && (oldGen < 0 || newGen >= oldGen)) {
						if expectedTurnID != "" && statusTurn != expectedTurnID {
							turnProgressSeen = false
						}
						expectedTurnID = statusTurn
					}
				}
			case queryStarted && statusState == "stopped":
				stopTurn := soothe.FrameTurnID(frame)
				if expectedTurnID != "" && !soothe.TurnIDsMatch(expectedTurnID, stopTurn) {
					continue
				}
				s.LastTurnEndState = statusState
				s.drainAfterIdle(ctx, expectedLoopID, out)
				return nil
			case queryStarted && statusState == "idle":
				idleTurn := soothe.FrameTurnID(frame)
				if !soothe.IsIdleTerminalAllowed(
					expectedTurnID, idleTurn, queryStarted, turnProgressSeen, s.LastTurnCancelSeen,
				) {
					continue
				}
				s.LastTurnEndState = statusState
				s.drainAfterIdle(ctx, expectedLoopID, out)
				return nil
			}
			continue
		}

		if eventType == "command_response" {
			content := asString(frame["content"])
			if strings.Contains(content, "Cancellation requested") {
				s.LastTurnCancelSeen = true
			}
			continue
		}

		if eventType != "event" {
			continue
		}

		data := frame["data"]
		namespace := asNamespace(frame["namespace"])
		mode := asString(frame["mode"])
		if s.earlyDropFn != nil && s.earlyDropFn(namespace, mode, data) {
			s.TurnEventStats.FilteredEarly++
			continue
		}

		if mode == "custom" && soothe.IsTurnEndCustomData(data) {
			dataTurn := evTurnID
			if m, ok := data.(map[string]interface{}); ok {
				if tid := soothe.FrameTurnID(m); tid != "" {
					dataTurn = tid
				}
			}
			if !soothe.IsTurnTerminalAllowed(expectedTurnID, dataTurn, queryStarted, turnProgressSeen) {
				continue
			}
		}

		if soothe.IsTurnProgressChunk(mode, data) {
			turnProgressSeen = true
		}

		select {
		case out <- TurnChunk{Namespace: namespace, Mode: mode, Data: data}:
		case <-ctx.Done():
			s.LastTurnErrorMessage = ctx.Err().Error()
			return ctx.Err()
		}

		if mode == "custom" && soothe.IsTurnEndCustomData(data) {
			customType := ""
			if m, ok := data.(map[string]interface{}); ok {
				customType = asString(m["type"])
			}
			if customType == soothe.STREAM_END {
				s.LastTurnEndState = "stream_end"
			} else {
				s.LastTurnEndState = "completed"
			}
			s.drainAfterIdle(ctx, expectedLoopID, out)
			return nil
		}
	}
}

func (s *DaemonSession) drainAfterIdle(ctx context.Context, expectedLoopID string, out chan<- TurnChunk) {
	deadline := time.Now().Add(s.postIdleDrain)
	exp := expectedLoopID
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return
		}
		ev, err := s.client.ReadEventWithTimeout(250 * time.Millisecond)
		if err != nil || ev == nil {
			return
		}
		frame := ev
		eventType := asString(frame["type"])
		if eventType == "next" {
			frame = UnwrapNext(frame)
			eventType = asString(frame["type"])
		}
		eventLoopID := asString(frame["loop_id"])
		if exp != "" && eventLoopID != "" && eventLoopID != exp {
			continue
		}
		if eventType == "error" {
			return
		}
		if eventType == "status" {
			if lid := asString(frame["loop_id"]); lid != "" {
				s.mu.Lock()
				s.loopID = lid
				s.mu.Unlock()
				exp = lid
			}
			continue
		}
		if eventType != "event" {
			continue
		}
		data := frame["data"]
		namespace := asNamespace(frame["namespace"])
		mode := asString(frame["mode"])
		if s.earlyDropFn != nil && s.earlyDropFn(namespace, mode, data) {
			s.TurnEventStats.FilteredEarly++
			continue
		}
		s.TurnEventStats.PostIdleDrained++
		select {
		case out <- TurnChunk{Namespace: namespace, Mode: mode, Data: data}:
		case <-ctx.Done():
			return
		}
	}
}
