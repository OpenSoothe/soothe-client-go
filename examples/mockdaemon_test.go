package soothe_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
)

// ---------------------------------------------------------------------------
// Mock daemon simulation layer (NJN-05)
// ---------------------------------------------------------------------------
//
// This file provides a reusable in-process mock Soothe daemon backed by
// httptest.NewServer + gorilla/websocket. It implements enough of the
// protocol-1 wire surface for Example_* functions to connect, bootstrap
// loops, invoke RPCs, and exercise the full client API without a live
// daemon. Every Example that previously hung on ws://localhost:8765 now
// spins up a NewMockDaemon, connects to its URL, and produces deterministic
// output.
//
// Usage in Example functions:
//
//	md := NewMockDaemon(nil)   // nil *testing.T is fine in examples
//	defer md.Close()
//	client := soothe.NewClient(md.URL, nil)
//	client.Connect(ctx)
//
// NewMockDaemon also accepts a *testing.T for cleanup registration when used
// from regular Test functions.

// TestMain is the entry point for the examples test binary. It exists so the
// package compiles as a test binary; no special setup is required.
func TestMain(m *testing.M) { m.Run() }

// MockDaemon is an in-process WebSocket server that speaks protocol-1.
type MockDaemon struct {
	// URL is the ws:// address to pass to soothe.NewClient.
	URL string

	server   *httptest.Server
	mu       sync.Mutex
	conns    []*websocket.Conn
	upgrader websocket.Upgrader

	// loopCounter generates deterministic loop IDs: loop-1, loop-2, ...
	loopCounter int
	// jobCounter generates deterministic job IDs: job-1, job-2, ...
	jobCounter int
	// cronCounter generates deterministic cron job IDs: cron-1, ...
	cronCounter int
}

// NewMockDaemon starts an in-process mock daemon and returns it.
// Pass a *testing.T to auto-register cleanup (Test functions), or nil to
// manage cleanup manually via defer md.Close() (Example functions).
func NewMockDaemon(t *testing.T) *MockDaemon {
	md := &MockDaemon{upgrader: websocket.Upgrader{}}
	md.server = httptest.NewServer(http.HandlerFunc(md.handle))
	md.URL = "ws" + strings.TrimPrefix(md.server.URL, "http")
	if t != nil {
		t.Cleanup(md.Close)
	}
	return md
}

// Close shuts down the mock daemon and all connections.
func (md *MockDaemon) Close() {
	md.server.Close()
	md.mu.Lock()
	defer md.mu.Unlock()
	for _, c := range md.conns {
		_ = c.Close()
	}
}

// handle is the WebSocket upgrade entry point.
func (md *MockDaemon) handle(w http.ResponseWriter, r *http.Request) {
	conn, err := md.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	md.mu.Lock()
	md.conns = append(md.conns, conn)
	md.mu.Unlock()
	defer func() { _ = conn.Close() }()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var m map[string]interface{}
		if err := json.Unmarshal(msg, &m); err != nil {
			continue
		}
		md.handleMessage(conn, m)
	}
}

// handleMessage dispatches a single protocol-1 envelope.
func (md *MockDaemon) handleMessage(conn *websocket.Conn, m map[string]interface{}) {
	// connection_init → handshake (status + connection_ack).
	if isConnInit(m) {
		md.sendHandshake(conn, m)
		return
	}

	typ, _ := m["type"].(string)
	method, _ := m["method"].(string)
	id, _ := m["id"].(string)
	params, _ := m["params"].(map[string]interface{})
	if params == nil {
		params = map[string]interface{}{}
	}

	switch {
	// --- Notifications (fire-and-forget) ----------------------------------
	case typ == "notification":
		// loop_input, slash_command, disconnect — acknowledge with a status.
		md.sendStatus(conn, "running", params["loop_id"])
		return

	// --- Subscriptions -----------------------------------------------------
	case typ == "subscribe" && method == "loop_events":
		md.sendNext(conn, id, map[string]interface{}{
			"loop_id":   params["loop_id"],
			"event":     "subscribed",
			"success":   true,
			"client_id": "mock-client",
		})
		return
	case typ == "subscribe" && method == "autopilot_events":
		md.sendNext(conn, id, map[string]interface{}{
			"event":   "subscribed",
			"success": true,
		})
		return
	case typ == "unsubscribe":
		md.sendResponse(conn, id, map[string]interface{}{"status": "unsubscribed"})
		return

	// --- Request-response RPCs -------------------------------------------
	case typ == "request" || typ == "subscribe":
		md.handleRPC(conn, id, method, params)
		return
	}
}

// handleRPC responds to protocol-1 request envelopes by method name.
func (md *MockDaemon) handleRPC(conn *websocket.Conn, id, method string, params map[string]interface{}) {
	switch method {
	// --- Loop lifecycle ---------------------------------------------------
	case "loop_new":
		md.sendResponse(conn, id, map[string]interface{}{
			"loop_id": md.nextLoopID(),
			"success": true,
		})
	case "loop_list":
		md.sendResponse(conn, id, map[string]interface{}{
			"loops": []interface{}{
				map[string]interface{}{"loop_id": "loop-1", "state": "idle"},
				map[string]interface{}{"loop_id": "loop-2", "state": "running"},
			},
			"total": 2,
		})
	case "loop_get":
		md.sendResponse(conn, id, map[string]interface{}{
			"loop_id": params["loop_id"],
			"state":   "idle",
			"active":  true,
		})
	case "loop_tree":
		md.sendResponse(conn, id, map[string]interface{}{
			"loop_id": params["loop_id"],
			"format":  params["format"],
			"tree":    "root\n  ├── checkpoint-1\n  └── checkpoint-2",
		})
	case "loop_prune":
		md.sendResponse(conn, id, map[string]interface{}{
			"loop_id":   params["loop_id"],
			"pruned":    3,
			"remaining": 2,
			"dry_run":   params["dry_run"],
		})
	case "loop_delete":
		md.sendResponse(conn, id, map[string]interface{}{
			"loop_id": params["loop_id"],
			"deleted": true,
		})
	case "loop_reattach":
		md.sendResponse(conn, id, map[string]interface{}{
			"loop_id": params["loop_id"],
			"success": true,
		})
	case "loop_input":
		md.sendResponse(conn, id, map[string]interface{}{
			"loop_id":  params["loop_id"],
			"accepted": true,
		})
	case "loop_messages":
		md.sendResponse(conn, id, map[string]interface{}{
			"loop_id": params["loop_id"],
			"messages": []interface{}{
				map[string]interface{}{"role": "user", "content": "Hello"},
				map[string]interface{}{"role": "assistant", "content": "Hi there"},
			},
			"total": 2,
		})
	case "loop_state_get":
		md.sendResponse(conn, id, map[string]interface{}{
			"loop_id": params["loop_id"],
			"state":   map[string]interface{}{"messages": []interface{}{}},
		})
	case "loop_state_update":
		md.sendResponse(conn, id, map[string]interface{}{
			"loop_id": params["loop_id"],
			"updated": true,
		})
	case "loop_history_fetch":
		md.sendResponse(conn, id, map[string]interface{}{
			"loop_id":    params["loop_id"],
			"history":    []interface{}{},
			"replayable": true,
		})

	// --- Autopilot jobs ---------------------------------------------------
	case "job_create":
		md.sendResponse(conn, id, map[string]interface{}{
			"job_id":  md.nextJobID(),
			"status":  "queued",
			"success": true,
		})
	case "job_status":
		md.sendResponse(conn, id, map[string]interface{}{
			"job_id":      params["job_id"],
			"goal_status": "in_progress",
			"workers":     3,
			"completed":   0,
			"total":       5,
		})
	case "job_pause":
		md.sendResponse(conn, id, map[string]interface{}{
			"job_id":  params["job_id"],
			"paused":  true,
			"success": true,
		})
	case "job_resume":
		md.sendResponse(conn, id, map[string]interface{}{
			"job_id":  params["job_id"],
			"resumed": true,
			"success": true,
		})
	case "job_cancel":
		md.sendResponse(conn, id, map[string]interface{}{
			"job_id":    params["job_id"],
			"cancelled": true,
			"success":   true,
		})
	case "job_dag":
		md.sendResponse(conn, id, map[string]interface{}{
			"job_id": params["job_id"],
			"dag":    "root_goal\n  ├── subtask_1\n  ├── subtask_2\n  └── subtask_3",
		})
	case "job_guidance":
		md.sendResponse(conn, id, map[string]interface{}{
			"job_id":   params["job_id"],
			"absorbed": true,
			"success":  true,
		})

	// --- Autopilot goal RPCs ---------------------------------------------
	case "autopilot_status":
		md.sendResponse(conn, id, map[string]interface{}{
			"state":     "active",
			"running":   true,
			"dreaming":  false,
			"loop_pool": map[string]interface{}{"active": 1, "idle": 2},
		})
	case "autopilot_submit":
		md.sendResponse(conn, id, map[string]interface{}{
			"status":  "submitted",
			"goal_id": md.nextJobID(),
		})
	case "autopilot_list_goals":
		md.sendResponse(conn, id, map[string]interface{}{
			"goals":  []interface{}{},
			"source": "autopilot_service",
		})
	case "autopilot_get_goal":
		md.sendResponse(conn, id, map[string]interface{}{
			"goal":   map[string]interface{}{"id": params["goal_id"], "status": "active"},
			"source": "autopilot_service",
		})
	case "autopilot_cancel_goal":
		md.sendResponse(conn, id, map[string]interface{}{
			"status":     "cancelled",
			"goal_id":    params["goal_id"],
			"new_status": "cancelled",
		})
	case "autopilot_cancel_all":
		md.sendResponse(conn, id, map[string]interface{}{
			"status":          "cancelled",
			"cancelled_count": 0,
			"goal_ids":        []interface{}{},
		})
	case "autopilot_wake":
		md.sendResponse(conn, id, map[string]interface{}{"status": "wake_sent"})
	case "autopilot_dream":
		md.sendResponse(conn, id, map[string]interface{}{"status": "dream_sent"})
	case "autopilot_resume":
		md.sendResponse(conn, id, map[string]interface{}{
			"status":     "reactivated",
			"goal_id":    params["goal_id"],
			"new_status": "pending",
		})
	case "autopilot_list_jobs":
		md.sendResponse(conn, id, map[string]interface{}{
			"jobs":   []interface{}{},
			"source": "autopilot_service",
		})
	case "autopilot_get_job":
		md.sendResponse(conn, id, map[string]interface{}{
			"job":             map[string]interface{}{"id": params["job_id"], "status": "active"},
			"dag":             map[string]interface{}{"nodes": []interface{}{}},
			"active_goals":    0,
			"completed_goals": 0,
			"total_goals":     0,
			"source":          "autopilot_service",
		})
	case "autopilot_top":
		md.sendResponse(conn, id, map[string]interface{}{
			"running":      true,
			"dreaming":     false,
			"loop_pool":    map[string]interface{}{"active": 0, "idle": 0, "total": 0, "max": 4},
			"generated_at": "2026-08-04T00:00:00Z",
			"jobs":         []interface{}{},
		})

	// --- Cron -------------------------------------------------------------
	case "cron_add":
		md.sendResponse(conn, id, map[string]interface{}{
			"job_id":  md.nextCronID(),
			"status":  "active",
			"success": true,
		})
	case "cron_list":
		md.sendResponse(conn, id, map[string]interface{}{
			"jobs": []interface{}{
				map[string]interface{}{"job_id": "cron-1", "text": "morning summary", "status": "active"},
			},
			"total": 1,
		})
	case "cron_show":
		md.sendResponse(conn, id, map[string]interface{}{
			"job_id": params["job_id"],
			"text":   "morning summary",
			"status": "active",
		})
	case "cron_cancel":
		md.sendResponse(conn, id, map[string]interface{}{
			"job_id":    params["job_id"],
			"cancelled": true,
			"success":   true,
		})

	// --- Daemon / config / skills / models --------------------------------
	case "daemon_status":
		md.sendResponse(conn, id, map[string]interface{}{
			"running":      true,
			"port_live":    true,
			"active_loops": 2,
		})
	case "daemon_shutdown":
		md.sendResponse(conn, id, map[string]interface{}{"status": "acknowledged"})
	case "config_get":
		section, _ := params["section"].(string)
		md.sendResponse(conn, id, map[string]interface{}{
			section: map[string]interface{}{"key": "value"},
		})
	case "config_reload":
		md.sendResponse(conn, id, map[string]interface{}{"status": "reloaded"})
	case "skills_list":
		md.sendResponse(conn, id, map[string]interface{}{
			"skills": []interface{}{
				map[string]interface{}{"name": "research", "description": "Research skill"},
				map[string]interface{}{"name": "browser", "description": "Browser skill"},
				map[string]interface{}{"name": "code_reviewer", "description": "Code review skill"},
			},
		})
	case "models_list":
		md.sendResponse(conn, id, map[string]interface{}{
			"models": []interface{}{
				map[string]interface{}{"id": "openai:gpt-4o", "provider": "openai"},
				map[string]interface{}{"id": "anthropic:claude-sonnet-4", "provider": "anthropic"},
			},
		})
	case "invoke_skill":
		md.sendResponse(conn, id, map[string]interface{}{
			"echo": map[string]interface{}{
				"skill":  params["skill"],
				"status": "ok",
			},
		})
	case "mcp_status":
		md.sendResponse(conn, id, map[string]interface{}{
			"servers": []interface{}{
				map[string]interface{}{"name": "filesystem", "status": "running"},
			},
			"active": 1,
		})

	// --- Auth -------------------------------------------------------------
	case "auth":
		md.sendResponse(conn, id, map[string]interface{}{
			"access_token":  "mock-access-token",
			"refresh_token": "mock-refresh-token",
			"expires_in":    3600,
			"success":       true,
		})
	case "auth_refresh":
		md.sendResponse(conn, id, map[string]interface{}{
			"access_token":  "mock-access-token-2",
			"refresh_token": "mock-refresh-token-2",
			"expires_in":    3600,
			"success":       true,
		})

	// --- Commands --------------------------------------------------------
	case "command_request":
		md.handleCommand(conn, id, params)

	default:
		// Unknown method: echo it back so the caller sees something.
		md.sendResponse(conn, id, map[string]interface{}{"echoed": method})
	}
}

// handleCommand responds to command_request envelopes by command name.
func (md *MockDaemon) handleCommand(conn *websocket.Conn, id string, params map[string]interface{}) {
	cmd, _ := params["command"].(string)
	switch cmd {
	case "memory":
		md.sendResponse(conn, id, map[string]interface{}{
			"token_usage":    1500,
			"context_window": 128000,
			"used":           1500,
			"limit":          128000,
		})
	case "history":
		md.sendResponse(conn, id, map[string]interface{}{
			"history": []interface{}{
				"What is a goroutine?",
				"How do channels work?",
				"Explain the sync package",
			},
		})
	case "plan":
		md.sendResponse(conn, id, map[string]interface{}{
			"plan": "1. Understand the question\n2. Gather context\n3. Synthesize answer",
		})
	case "review":
		md.sendResponse(conn, id, map[string]interface{}{
			"review": "The conversation covers Go concurrency topics.",
		})
	case "dashboard", "autopilot_dashboard":
		md.sendResponse(conn, id, map[string]interface{}{
			"jobs":      3,
			"active":    1,
			"paused":    0,
			"completed": 2,
		})
	case "policy":
		md.sendResponse(conn, id, map[string]interface{}{
			"policy":    "default",
			"max_turns": 20,
		})
	case "config":
		md.sendResponse(conn, id, map[string]interface{}{
			"models": map[string]interface{}{"default": "openai:gpt-4o"},
		})
	case "clear", "cancel":
		md.sendResponse(conn, id, map[string]interface{}{"status": "cleared"})
	case "detach":
		md.sendResponse(conn, id, map[string]interface{}{"status": "detached"})
	case "exit", "quit":
		md.sendResponse(conn, id, map[string]interface{}{"status": "exit"})
	default:
		md.sendResponse(conn, id, map[string]interface{}{"status": "ok", "command": cmd})
	}
}

// --- Wire helpers -----------------------------------------------------------

func isConnInit(m map[string]interface{}) bool {
	t, _ := m["type"].(string)
	return t == "connection_init"
}

func (md *MockDaemon) sendHandshake(conn *websocket.Conn, m map[string]interface{}) {
	// Leading status frame.
	md.write(conn, map[string]interface{}{
		"proto":         "1",
		"type":          "status",
		"state":         "idle",
		"input_history": []interface{}{},
	})
	// connection_ack with negotiated capabilities.
	clientCaps, _ := m["capabilities"].([]interface{})
	daemonCaps := []string{"streaming", "batch", "heartbeat", "receipts"}
	capSet := map[string]bool{}
	for _, c := range clientCaps {
		if s, ok := c.(string); ok {
			capSet[s] = true
		}
	}
	negotiated := []string{}
	for _, c := range daemonCaps {
		if capSet[c] {
			negotiated = append(negotiated, c)
		}
	}
	md.write(conn, map[string]interface{}{
		"proto": "1",
		"type":  "connection_ack",
		"result": map[string]interface{}{
			"server_version":        "0.1.0",
			"protocol_version":      "1",
			"capabilities":          negotiated,
			"readiness_state":       "ready",
			"heartbeat_interval_ms": 0,
		},
	})
}

func (md *MockDaemon) sendResponse(conn *websocket.Conn, id string, result map[string]interface{}) {
	md.write(conn, map[string]interface{}{
		"proto":  "1",
		"type":   "response",
		"result": result,
		"id":     id,
	})
}

func (md *MockDaemon) sendNext(conn *websocket.Conn, id string, payload map[string]interface{}) {
	md.write(conn, map[string]interface{}{
		"proto":   "1",
		"type":    "next",
		"payload": payload,
		"id":      id,
	})
}

func (md *MockDaemon) sendStatus(conn *websocket.Conn, state string, loopID interface{}) {
	env := map[string]interface{}{
		"proto": "1",
		"type":  "status",
		"state": state,
	}
	if loopID != nil {
		env["loop_id"] = loopID
	}
	md.write(conn, env)
}

func (md *MockDaemon) write(conn *websocket.Conn, v map[string]interface{}) {
	b, _ := json.Marshal(v)
	_ = conn.WriteMessage(websocket.TextMessage, b)
}

// --- Deterministic ID generators -------------------------------------------

func (md *MockDaemon) nextLoopID() string {
	md.mu.Lock()
	defer md.mu.Unlock()
	md.loopCounter++
	return "loop-" + itoa(md.loopCounter)
}

func (md *MockDaemon) nextJobID() string {
	md.mu.Lock()
	defer md.mu.Unlock()
	md.jobCounter++
	return "job-" + itoa(md.jobCounter)
}

func (md *MockDaemon) nextCronID() string {
	md.mu.Lock()
	defer md.mu.Unlock()
	md.cronCounter++
	return "cron-" + itoa(md.cronCounter)
}

// itoa is a strconv-free int-to-string for small numbers.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
