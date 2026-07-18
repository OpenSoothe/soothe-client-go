package appkit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	soothe "github.com/mirasoth/soothe-client-go"
)

// ---------------------------------------------------------------------------
// Test fakes
// ---------------------------------------------------------------------------

// memStore is an in-memory SessionStore for tests.
type memStore struct {
	mu         sync.Mutex
	sessions   map[string]*SessionEntry
	msgs       map[string][]SessionMessage
	failCreate bool
}

func newMemStore() *memStore {
	return &memStore{
		sessions: make(map[string]*SessionEntry),
		msgs:     make(map[string][]SessionMessage),
	}
}

func (s *memStore) GetSession(ctx context.Context, id string) (*SessionEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.sessions[id]; ok {
		cp := *e
		return &cp, nil
	}
	return nil, nil
}
func (s *memStore) CreateSession(ctx context.Context, ws, sid, loop, stype string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failCreate {
		return errors.New("create failed")
	}
	s.sessions[sid] = &SessionEntry{WorkspaceID: ws, AppKey: sid, LoopID: loop, SessionType: stype, IsActive: true}
	return nil
}
func (s *memStore) UpdateLastUsed(ctx context.Context, sid string) error      { return nil }
func (s *memStore) IncrementResetCount(ctx context.Context, sid string) error { return nil }
func (s *memStore) GetLoopIDForSession(ctx context.Context, sid string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.sessions[sid]; ok && e.LoopID != "" {
		return e.LoopID, true, nil
	}
	return "", false, nil
}
func (s *memStore) AppendMessage(ctx context.Context, sid string, m SessionMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs[sid] = append(s.msgs[sid], m)
	return nil
}
func (s *memStore) messages(sid string) []SessionMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]SessionMessage, len(s.msgs[sid]))
	copy(cp, s.msgs[sid])
	return cp
}

// fakeClient is a ManagedClient fake that handshakes on Connect and replays
// a scripted event stream on ReceiveMessages.
type fakeClient struct {
	mu               sync.Mutex
	connected        bool
	closed           bool
	disconnCh        chan soothe.DisconnectCause
	scripted         []interface{} // events to emit on the reader channel
	closeAfterScript bool          // if true, close ch after script (stream-close tests)
	sendCapture      []map[string]interface{}
	reattachErr      error
	connectErr       error
}

func newFakeClient(events ...interface{}) *fakeClient {
	return &fakeClient{
		disconnCh: make(chan soothe.DisconnectCause, 1),
		scripted:  events,
	}
}

func newFakeClientCloseAfter(events ...interface{}) *fakeClient {
	f := newFakeClient(events...)
	f.closeAfterScript = true
	return f
}

func (f *fakeClient) Connect(ctx context.Context) error {
	f.mu.Lock()
	f.connected = true
	f.mu.Unlock()
	return f.connectErr
}
func (f *fakeClient) Reconnect(ctx context.Context) error {
	f.mu.Lock()
	f.connected = true
	f.mu.Unlock()
	return nil
}
func (f *fakeClient) ReattachAndProbe(ctx context.Context, id string) error { return f.reattachErr }
func (f *fakeClient) SendMessage(ctx context.Context, msg interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if m, ok := msg.(map[string]interface{}); ok {
		f.sendCapture = append(f.sendCapture, m)
	}
	return nil
}
func (f *fakeClient) ReceiveMessages(ctx context.Context) (<-chan interface{}, error) {
	ch := make(chan interface{}, 32)
	go func() {
		defer close(ch)
		for _, ev := range f.scripted {
			select {
			case ch <- ev:
			case <-ctx.Done():
				return
			}
		}
		if f.closeAfterScript {
			return
		}
		// Hold the channel open until cancelled so the turn loop doesn't see
		// a premature close; the deliverable event ends the turn first.
		<-ctx.Done()
	}()
	return ch, nil
}
func (f *fakeClient) Disconnected() <-chan soothe.DisconnectCause { return f.disconnCh }
func (f *fakeClient) IsConnected() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.connected && !f.closed
}
func (f *fakeClient) Close() error {
	f.mu.Lock()
	f.closed = true
	f.connected = false
	f.mu.Unlock()
	return nil
}
func (f *fakeClient) sentMessages() []map[string]interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]map[string]interface{}, len(f.sendCapture))
	copy(cp, f.sendCapture)
	return cp
}

// makeEnvelopeMessage builds an EventMessage-like frame via the soothe package's
// decode path so the classifier sees a real soothe.EventMessage.
func eventMessageFromJSON(t *testing.T, jsonStr string) interface{} {
	t.Helper()
	msg, err := soothe.DecodeMessage([]byte(jsonStr))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return msg
}

// deliverableEvent builds a loop-tagged assistant deliverable event for a phase.
func deliverableEvent(phase, content string) string {
	return fmt.Sprintf(`{"proto":"1","type":"event","namespace":["soothe","protocol","message"],"mode":"messages","data":[{"type":"AIMessage","phase":"%s","content":"%s"}],"loop_id":"loop-1"}`, phase, content)
}

// ---------------------------------------------------------------------------
// SSEBroadcaster
// ---------------------------------------------------------------------------

func TestSSEBroadcaster_SubscribeBroadcastClose(t *testing.T) {
	b := NewSSEBroadcaster()
	ch, _ := b.Subscribe("s1")
	b.Broadcast("s1", SSEEvent{Type: "delta", Data: "hi"})
	select {
	case ev := <-ch:
		if ev.Type != "delta" || ev.Data != "hi" {
			t.Errorf("unexpected event: %+v", ev)
		}
	case <-time.After(time.Second):
		t.Error("did not receive broadcast")
	}
	// Non-existent session: no panic, no delivery.
	b.Broadcast("nope", SSEEvent{Type: "x"})
	b.Close("s1")
	// Channel should be closed after Close.
	if _, ok := <-ch; ok {
		t.Error("expected channel closed after Close")
	}
}

func TestSSEBroadcaster_DropOnFull(t *testing.T) {
	b := NewSSEBroadcaster()
	ch, _ := b.Subscribe("s1")
	// Fill the buffer (cap 100).
	for i := 0; i < 100; i++ {
		b.Broadcast("s1", SSEEvent{Type: "delta"})
	}
	// Next broadcast must not block (drop-on-full).
	done := make(chan struct{})
	go func() {
		b.Broadcast("s1", SSEEvent{Type: "delta"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("Broadcast blocked on full channel")
	}
	// Drain so Close doesn't leak goroutines.
	for i := 0; i < 100; i++ {
		<-ch
	}
	b.Close("s1")
}

// ---------------------------------------------------------------------------
// EventClassifier
// ---------------------------------------------------------------------------

func defaultClassifier() *EventClassifier {
	return NewEventClassifier(ClassifierConfig{
		DeliverablePhases: soothe.DefaultDeliverablePhases(),
	})
}

func TestEventClassifier_DeliverablePhase_DefaultSet(t *testing.T) {
	cl := defaultClassifier()
	msg := eventMessageFromJSON(t, deliverableEvent("quiz", "Hello, this is the answer."))
	r := cl.Classify(msg, "")
	if r.Terminal != ChatEventDeliverableComplete {
		t.Errorf("expected deliverable, got %v", r.Terminal)
	}
	if !strings.Contains(r.CompletionEvent, "quiz") {
		t.Errorf("expected completion event with quiz, got %s", r.CompletionEvent)
	}
}

func TestEventClassifier_PhaseNotInConfig_NotDeliverable(t *testing.T) {
	// An app that only treats direct_model as deliverable should not finish on quiz.
	cl := NewEventClassifier(ClassifierConfig{
		DeliverablePhases: map[string]bool{"direct_model": true},
	})
	msg := eventMessageFromJSON(t, deliverableEvent("quiz", "Hello, this is the answer."))
	r := cl.Classify(msg, "")
	if r.Terminal == ChatEventDeliverableComplete {
		t.Error("quiz should NOT be deliverable when not in the app's DeliverablePhases")
	}
}

func TestEventClassifier_StreamingChunk_Continue(t *testing.T) {
	cl := defaultClassifier()
	msg := eventMessageFromJSON(t, `{"proto":"1","type":"event","namespace":["soothe","protocol","message"],"mode":"messages","data":[{"type":"AIMessageChunk","content":"partial"}],"loop_id":"loop-1"}`)
	r := cl.Classify(msg, "")
	if r.Terminal != ChatEventContinue {
		t.Errorf("expected continue for streaming chunk, got %v", r.Terminal)
	}
	if r.Content != "partial" {
		t.Errorf("expected content 'partial', got %q", r.Content)
	}
}

func TestEventClassifier_SubstantiveReplyGuard(t *testing.T) {
	cl := defaultClassifier()
	// A short stub ACK ("...") in a deliverable phase should NOT be persisted.
	msg := eventMessageFromJSON(t, deliverableEvent("quiz", "..."))
	r := cl.Classify(msg, "")
	if r.Terminal == ChatEventDeliverableComplete {
		t.Error("stub ACK should not be deliverable (min-rune guard)")
	}
}

// ---------------------------------------------------------------------------
// QueryGate
// ---------------------------------------------------------------------------

func TestQueryGate_SingleFlight(t *testing.T) {
	g := NewQueryGate()
	cancel := func() {}
	if err := g.Acquire("s1", cancel, nil); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if err := g.Acquire("s1", func() {}, nil); err != ErrQueryBusy {
		t.Errorf("expected ErrQueryBusy, got %v", err)
	}
	g.Release("s1")
	// After release, acquire should succeed again.
	if err := g.Acquire("s1", cancel, nil); err != nil {
		t.Errorf("acquire after release: %v", err)
	}
	g.Release("s1")
}

func TestQueryGate_CancelOrdering(t *testing.T) {
	g := NewQueryGate()
	localCancelled := make(chan struct{})
	localCancel := func() { close(localCancelled) }

	daemonCancelCalled := make(chan struct{}, 1)
	sendCancel := func(ctx context.Context) error {
		close(daemonCancelCalled)
		return nil
	}
	_ = g.Acquire("s1", localCancel, sendCancel)

	_ = g.Cancel("s1")

	// Daemon cancel must be called before local cancel.
	select {
	case <-daemonCancelCalled:
	default:
		t.Error("expected daemon cancel to be called")
	}
	select {
	case <-localCancelled:
	default:
		t.Error("expected local cancel to be called after daemon cancel")
	}
}

// ---------------------------------------------------------------------------
// ConnectionPool
// ---------------------------------------------------------------------------

// poolTestHarness wires a pool with a fake factory + bootstrap.
func newTestPool(t *testing.T, store *memStore, fake *fakeClient) *ConnectionPool {
	t.Helper()
	factory := func(url string, cfg *soothe.Config) ManagedClient { return fake }
	return NewConnectionPool(
		"ws://test",
		store,
		&PoolConfig{PoolSize: 4, QueryTimeout: 5 * time.Second},
		nil,
		factory,
	).WithBootstrap(func(ctx context.Context, c ManagedClient, ws, uid string, cfg *soothe.Config) (string, error) {
		return "loop-fresh", nil
	})
}

func TestConnectionPool_PreSeeded(t *testing.T) {
	store := newMemStore()
	pool := newTestPool(t, store, newFakeClient())
	active, idle := pool.Stats()
	if active != 0 || idle != 4 {
		t.Fatalf("expected 0 active and 4 idle slots, got active=%d idle=%d", active, idle)
	}
}

func TestConnectionPool_Exhausted(t *testing.T) {
	store := newMemStore()
	pool := NewConnectionPool(
		"ws://test",
		store,
		&PoolConfig{PoolSize: 1, QueryTimeout: time.Second},
		nil,
		func(url string, cfg *soothe.Config) ManagedClient { return newFakeClient() },
	).WithBootstrap(func(ctx context.Context, c ManagedClient, ws, uid string, cfg *soothe.Config) (string, error) {
		return "loop-1", nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if _, err := pool.Acquire(ctx, "s1", "ws-1", "user-1"); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if _, err := pool.Acquire(ctx, "s2", "ws-1", "user-1"); !errors.Is(err, ErrPoolExhausted) {
		t.Fatalf("expected ErrPoolExhausted, got %v", err)
	}
}

func TestConnectionPool_BootstrapNewSession(t *testing.T) {
	store := newMemStore()
	fake := newFakeClient()
	pool := newTestPool(t, store, fake)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := pool.Acquire(ctx, "s1", "ws-1", "user-1")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer pool.Release("s1")
	if conn.getLoopID() != "loop-fresh" {
		t.Errorf("expected loop-fresh, got %s", conn.getLoopID())
	}
	// Session should be persisted.
	e, _ := store.GetSession(ctx, "s1")
	if e == nil || e.LoopID != "loop-fresh" {
		t.Errorf("session not persisted correctly: %+v", e)
	}
}

func TestConnectionPool_ReuseActiveConnection(t *testing.T) {
	store := newMemStore()
	fake := newFakeClient()
	pool := newTestPool(t, store, fake)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn1, err := pool.Acquire(ctx, "s1", "ws-1", "user-1")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	_ = conn1
	// Second acquire on the same session should reuse, not re-bootstrap.
	conn2, err := pool.Acquire(ctx, "s1", "ws-1", "user-1")
	if err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	if conn2 != conn1 {
		t.Error("expected the same connection on reuse")
	}
}

// ---------------------------------------------------------------------------
// TurnRunner (end-to-end scripted)
// ---------------------------------------------------------------------------

func TestTurnRunner_DeliverableTurn(t *testing.T) {
	store := newMemStore()
	deliverable := eventMessageFromJSON(t, deliverableEvent("text_completion", "This is a substantive final answer."))
	fake := newFakeClient(deliverable)
	pool := newTestPool(t, store, fake)
	gate := NewQueryGate()
	cl := defaultClassifier()
	b := NewSSEBroadcaster()
	tr := NewTurnRunner(pool, gate, cl, store, b, TurnConfig{QueryTimeout: 2 * time.Second})

	sub, _ := b.Subscribe("s1")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := tr.Execute(ctx, "s1", "what is 2+2", "user-1", "ws-1", nil, &InputOpts{IntentHint: soothe.IntentHintTextCompletion}); err != nil {
		t.Fatalf("execute: %v", err)
	}

	// The deliverable event's content is streamed as a delta before the
	// turn completes, so drain events until the "complete" arrives.
	var got SSEEvent
	var deltas []string
	for {
		select {
		case got = <-sub:
		case <-time.After(time.Second):
			t.Fatal("no complete event received")
		}
		if got.Type == "delta" {
			if s, ok := got.Data.(string); ok {
				deltas = append(deltas, s)
			}
			continue
		}
		break
	}
	if got.Type != "complete" {
		t.Errorf("expected complete, got %s", got.Type)
	}
	// The streamed delta should carry the deliverable content.
	if joined := strings.Join(deltas, ""); joined != "This is a substantive final answer." {
		t.Errorf("expected streamed deltas to carry the reply, got %q", joined)
	}
	// And the assistant reply persisted to the store.
	msgs := store.messages("s1")
	if len(msgs) == 0 || msgs[0].Role != "assistant" {
		t.Errorf("expected persisted assistant message, got %+v", msgs)
	}
	// And loop_input was sent with the intent hint.
	sent := fake.sentMessages()
	if len(sent) == 0 || sent[0]["type"] != "loop_input" || sent[0]["intent_hint"] != soothe.IntentHintTextCompletion {
		t.Errorf("unexpected send: %+v", sent)
	}
}

// streamingChunkEvent builds a mode=messages AIMessageChunk carrying one piece
// of streaming assistant text (classifier maps these to ChatEventContinue).
func streamingChunkEvent(content string) string {
	return fmt.Sprintf(`{"proto":"1","type":"event","namespace":["soothe","protocol","message"],"mode":"messages","data":[{"type":"AIMessageChunk","content":%q}],"loop_id":"loop-1"}`, content)
}

// drainUntil collects SSEEvents from sub until one of wantTypes arrives, then
// returns that event and every delta seen before it.
func drainUntil(t *testing.T, sub <-chan SSEEvent, wantTypes ...string) (final SSEEvent, deltas []string) {
	t.Helper()
	want := make(map[string]bool, len(wantTypes))
	for _, w := range wantTypes {
		want[w] = true
	}
	for {
		select {
		case ev := <-sub:
			if ev.Type == "delta" {
				if s, ok := ev.Data.(string); ok {
					deltas = append(deltas, s)
				}
				continue
			}
			if want[ev.Type] {
				return ev, deltas
			}
		case <-time.After(time.Second):
			t.Fatalf("drainUntil: no %v event received", wantTypes)
		}
	}
}

// TestTurnRunner_StreamsContentDeltas verifies that streaming chunks are
// emitted as "delta" events whose concatenation equals the final "complete".
func TestTurnRunner_StreamsContentDeltas(t *testing.T) {
	store := newMemStore()
	chunk1 := eventMessageFromJSON(t, streamingChunkEvent("Hello "))
	chunk2 := eventMessageFromJSON(t, streamingChunkEvent("world"))
	final := eventMessageFromJSON(t, deliverableEvent("text_completion", "Hello world"))
	fake := newFakeClient(chunk1, chunk2, final)
	pool := newTestPool(t, store, fake)
	gate := NewQueryGate()
	cl := defaultClassifier()
	b := NewSSEBroadcaster()
	tr := NewTurnRunner(pool, gate, cl, store, b, TurnConfig{QueryTimeout: 2 * time.Second})

	sub, _ := b.Subscribe("s1")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := tr.Execute(ctx, "s1", "hi", "user-1", "ws-1", nil, nil); err != nil {
		t.Fatalf("execute: %v", err)
	}

	got, deltas := drainUntil(t, sub, "complete")
	if got.Type != "complete" {
		t.Errorf("expected complete, got %s", got.Type)
	}
	if joined := strings.Join(deltas, ""); joined != "Hello world" {
		t.Errorf("expected streamed deltas 'Hello world', got %q", joined)
	}
	if got.Data != "Hello world" {
		t.Errorf("expected complete data 'Hello world', got %v", got.Data)
	}
}

// TestTurnRunner_CumulativeContent verifies that when the daemon sends the
// full-so-far text on each event (cumulative), only the new suffix is streamed.
func TestTurnRunner_CumulativeContent(t *testing.T) {
	store := newMemStore()
	// Two cumulative AIMessages in a deliverable phase: the classifier treats
	// a deliverable-phase AIMessage as deliverable, so use non-deliverable
	// phase "chunk" (continue) then a final text_completion deliverable.
	c1 := eventMessageFromJSON(t, deliverableEvent("chunk", "Hi"))
	c2 := eventMessageFromJSON(t, deliverableEvent("chunk", "Hi there"))
	final := eventMessageFromJSON(t, deliverableEvent("text_completion", "Hi there"))
	fake := newFakeClient(c1, c2, final)
	pool := newTestPool(t, store, fake)
	gate := NewQueryGate()
	cl := defaultClassifier()
	b := NewSSEBroadcaster()
	tr := NewTurnRunner(pool, gate, cl, store, b, TurnConfig{QueryTimeout: 2 * time.Second})

	sub, _ := b.Subscribe("s1")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := tr.Execute(ctx, "s1", "hi", "user-1", "ws-1", nil, nil); err != nil {
		t.Fatalf("execute: %v", err)
	}

	_, deltas := drainUntil(t, sub, "complete")
	// Cumulative trim-prefix: "Hi" emits "Hi"; "Hi there" emits only the new
	// suffix " there"; the final deliverable "Hi there" is a prefix of the
	// accumulated text and so emits no delta (dedup). Net streamed text is the
	// reply itself, with no duplicate re-emission at completion.
	want := "Hi there"
	if joined := strings.Join(deltas, ""); joined != want {
		t.Errorf("expected cumulative deltas %q, got %q", want, joined)
	}
}

func TestTurnRunner_Timeout(t *testing.T) {
	store := newMemStore()
	// No deliverable event — the turn will run until timeout.
	fake := newFakeClient( /* no events that complete */ )
	pool := newTestPool(t, store, fake)
	gate := NewQueryGate()
	cl := defaultClassifier()
	tr := NewTurnRunner(pool, gate, cl, store, NewSSEBroadcaster(), TurnConfig{QueryTimeout: 100 * time.Millisecond})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := tr.Execute(ctx, "s1", "stalled", "user-1", "ws-1", nil, nil)
	if !errors.Is(err, ErrQueryTimeout) {
		t.Errorf("expected ErrQueryTimeout, got %v", err)
	}
	// Expect an error row persisted.
	msgs := store.messages("s1")
	if len(msgs) == 0 || msgs[0].Role != "error" {
		t.Errorf("expected persisted error message, got %+v", msgs)
	}
}
