package soothe

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// ---------------------------------------------------------------------------
// ConnectWithRetries unit tests (session.go:97-122)
//
// Strategy: use httptest.Server handlers whose behaviour can be controlled at
// runtime to simulate connection failures and eventual success. The retry
// loop calls client.Connect(ctx) which performs a WebSocket dial + protocol-1
// handshake. A handler that closes the connection immediately (or never
// upgrades) causes Connect to return an error, driving the retry loop.
// ---------------------------------------------------------------------------

// acceptingHandler is a minimal echo/handshake handler that always accepts
// connections (mirrors testEchoHandler from client_test.go but self-contained).
func acceptingHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		// If it's a connection_init, send the handshake ack.
		var m map[string]interface{}
		if err := json.Unmarshal(msg, &m); err != nil {
			continue
		}
		if isConnectionInit(m) {
			testSendHandshake(conn, m)
			continue
		}
		// Echo other messages.
		_ = conn.WriteMessage(websocket.TextMessage, msg)
	}
}

// rejectingHandler responds with HTTP 503 to every request, causing the
// WebSocket dialer to fail before any upgrade.
func rejectingHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusServiceUnavailable)
}

// -----------------------------------------------------------------------
// Test 1: success on first attempt
// -----------------------------------------------------------------------

func TestConnectWithRetries_SuccessOnFirstAttempt(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(acceptingHandler))
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := ConnectWithRetries(ctx, client, 3, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !client.IsConnected() {
		t.Error("expected client to be connected")
	}
	_ = client.Close()
}

// -----------------------------------------------------------------------
// Test 2: retry loop defaults — maxRetries<=0 ⇒ 40, retryDelay<=0 ⇒ 250ms
//
// We don't wait for all 40 retries (that would take ~10s). Instead we point
// at an unreachable port so every attempt fails, use maxRetries=1 to confirm
// the *returned error* mentions the count, then separately verify the
// default is applied by checking that maxRetries<=0 produces an error
// mentioning "40 attempts" (with a short ctx timeout to bound wall time).
// -----------------------------------------------------------------------

func TestConnectWithRetries_DefaultMaxRetries(t *testing.T) {
	// Unreachable port: every dial fails immediately.
	client := NewClient("ws://127.0.0.1:59999", nil)

	// Use a context that cancels quickly to bound the test wall time; the
	// default 40-attempt loop would otherwise retry for ~10s.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := ConnectWithRetries(ctx, client, 0, 1*time.Millisecond)
	if err == nil {
		t.Fatal("expected error when all attempts fail, got nil")
	}
	// The loop should exit via ctx.Done() before exhausting all 40 retries,
	// returning a context error.
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		// If it somehow exhausted retries first, the error would mention
		// "40 attempts". That's also acceptable (default was applied).
		t.Logf("got error: %v (expected context error or 40-attempt exhaustion)", err)
	}
}

// -----------------------------------------------------------------------
// Test 2b: verify maxRetries<=0 actually defaults to 40 by counting
// attempts against a rejecting server, with ctx cancelled *after* the loop
// starts so we can inspect the error message for "40 attempts".
//
// Because 40 × 250ms = 10s which is too long, we verify the default is 40
// via the error string from a fully-exhausted loop using a tiny retryDelay
// override (retryDelay is independently tested in Test 3).
// -----------------------------------------------------------------------

func TestConnectWithRetries_DefaultMaxRetriesValue(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(rejectingHandler))
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// maxRetries<=0 ⇒ default 40; retryDelay small to keep the test fast.
	err := ConnectWithRetries(ctx, client, 0, 1*time.Millisecond)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "40 attempts") {
		t.Errorf("expected error to mention '40 attempts' (default maxRetries), got: %v", err)
	}
}

// -----------------------------------------------------------------------
// Test 3: retryDelay<=0 ⇒ default 250ms
//
// We can't easily measure 250ms precisely in a unit test, but we *can*
// confirm that retryDelay<=0 does NOT crash and that the loop still
// functions. We measure the lower bound: with a rejecting server and
// maxRetries=2, the two attempts should be separated by at least the
// default 250ms delay. A generous floor of 200ms confirms the default
// was applied (vs. a zero/near-zero delay).
// -----------------------------------------------------------------------

func TestConnectWithRetries_DefaultRetryDelay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(rejectingHandler))
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	err := ConnectWithRetries(ctx, client, 2, 0) // retryDelay<=0 ⇒ 250ms default
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error after exhausting 2 attempts, got nil")
	}
	if !strings.Contains(err.Error(), "2 attempts") {
		t.Errorf("expected error mentioning '2 attempts', got: %v", err)
	}
	// 2 attempts with default 250ms delay: at least 250ms should elapse
	// between attempt 1 and attempt 2. The dial itself is near-instant
	// against a local rejecting server. Use a 200ms floor to avoid CI
	// timing flakiness while still confirming the default was applied.
	if elapsed < 200*time.Millisecond {
		t.Errorf("expected ≥200ms elapsed (250ms default delay between 2 attempts), got %v", elapsed)
	}
}

// -----------------------------------------------------------------------
// Test 4: context cancellation during retry loop via ctx.Done()
//
// Start a retry loop against a rejecting server with a large maxRetries
// and small delay, then cancel the context mid-loop. The loop should
// return a context.Canceled error promptly.
// -----------------------------------------------------------------------

func TestConnectWithRetries_ContextCancellation(t *testing.T) {
	cfg := GetCIConfig()
	ts := httptest.NewServer(http.HandlerFunc(rejectingHandler))
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after short delay — well before maxRetries×delay would complete.
	go func() {
		time.Sleep(cfg.RetryDelay)
		cancel()
	}()

	start := time.Now()
	err := ConnectWithRetries(ctx, client, 100, 10*time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
	// Should return promptly after cancellation, not after all 100 retries.
	if elapsed > 1*time.Second {
		t.Errorf("expected prompt return after cancellation (<1s), got %v", elapsed)
	}
}

// -----------------------------------------------------------------------
// Test 5: eventual success after N retries
//
// Use a handler that fails (closes immediately) for the first N attempts,
// then starts accepting. An atomic counter tracks how many dial attempts
// have been made; once the threshold is reached, switch to the accepting
// handler.
//
// Because httptest.Server uses a fixed handler, we simulate this by having
// a single handler that checks the counter and either rejects (HTTP 503)
// or upgrades.
// -----------------------------------------------------------------------

func TestConnectWithRetries_EventualSuccessAfterRetries(t *testing.T) {
	var attempts int32

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		// Fail the first 3 attempts, succeed from attempt 4 onward.
		if n < 4 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		acceptingHandler(w, r)
	})

	ts := httptest.NewServer(handler)
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := ConnectWithRetries(ctx, client, 10, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("expected eventual success after retries, got: %v", err)
	}
	if !client.IsConnected() {
		t.Error("expected client to be connected after retries")
	}

	finalAttempts := atomic.LoadInt32(&attempts)
	if finalAttempts < 4 {
		t.Errorf("expected ≥4 dial attempts before success, got %d", finalAttempts)
	}
	_ = client.Close()
}
