package soothe_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// HTTP-REST autopilot examples (RFC-204)
// ---------------------------------------------------------------------------
//
// The Soothe daemon exposes autopilot endpoints ONLY via HTTP REST, not via
// the WebSocket protocol this client speaks. The endpoints are:
//
//   GET  /api/v1/autopilot/status          - Get autopilot state
//   GET  /api/v1/autopilot/goals           - List goals
//   POST /api/v1/autopilot/goals           - Submit a goal
//   POST /api/v1/autopilot/goals/{id}/approve - Approve a goal
//   POST /api/v1/autopilot/goals/{id}/reject  - Reject a goal
//   POST /api/v1/autopilot/wake            - Transition to wake mode
//   POST /api/v1/autopilot/dream           - Transition to dream mode
//
// These examples use httptest.NewServer to mock the REST responses, so they
// run without a live daemon and produce deterministic output.

// restAutopilotServer is a mock HTTP REST server for RFC-204 autopilot endpoints.
type restAutopilotServer struct {
	*httptest.Server
}

func newRestAutopilotServer() *restAutopilotServer {
	mux := http.NewServeMux()

	// GET /api/v1/autopilot/status
	mux.HandleFunc("/api/v1/autopilot/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"mode":         "idle",
			"active_goals": 2,
			"workers":      4,
			"queue_depth":  0,
		})
	})

	// GET/POST /api/v1/autopilot/goals
	mux.HandleFunc("/api/v1/autopilot/goals", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			json.NewEncoder(w).Encode(map[string]interface{}{
				"goals": []interface{}{
					map[string]interface{}{"id": "goal-1", "text": "Refactor auth module", "status": "in_progress"},
					map[string]interface{}{"id": "goal-2", "text": "Add API tests", "status": "pending"},
				},
				"total": 2,
			})
		case http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			var req map[string]interface{}
			json.Unmarshal(body, &req)
			text, _ := req["text"].(string)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "goal-new",
				"text":   text,
				"status": "submitted",
			})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// POST /api/v1/autopilot/goals/{id}/approve
	mux.HandleFunc("/api/v1/autopilot/goals/goal-1/approve", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// POST /api/v1/autopilot/goals/{id}/reject
	mux.HandleFunc("/api/v1/autopilot/goals/goal-1/reject", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// POST /api/v1/autopilot/wake
	mux.HandleFunc("/api/v1/autopilot/wake", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"mode":   "wake",
			"status": "transitioned",
		})
	})

	// POST /api/v1/autopilot/dream
	mux.HandleFunc("/api/v1/autopilot/dream", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"mode":   "dream",
			"status": "transitioned",
		})
	})

	ts := httptest.NewServer(mux)
	return &restAutopilotServer{Server: ts}
}

// restGet performs a GET request and returns the decoded JSON body.
func restGet(ctx context.Context, url string) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// restPost performs a POST request with an optional JSON body.
// If body is nil, sends an empty POST.
func restPost(ctx context.Context, url string, body map[string]interface{}) (map[string]interface{}, int, error) {
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, reader)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	if resp.StatusCode == http.StatusNoContent {
		return nil, resp.StatusCode, nil
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result, resp.StatusCode, nil
}

// Example_restAutopilotStatus demonstrates querying autopilot state via the
// HTTP REST endpoint GET /api/v1/autopilot/status (RFC-204).
func Example_restAutopilotStatus() {
	ts := newRestAutopilotServer()
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := restGet(ctx, ts.URL+"/api/v1/autopilot/status")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Mode: %v\n", result["mode"])
	fmt.Printf("Active goals: %v\n", result["active_goals"])
	fmt.Printf("Workers: %v\n", result["workers"])
	// Output:
	// Mode: idle
	// Active goals: 2
	// Workers: 4
}

// Example_restAutopilotListGoals lists autopilot goals via the HTTP REST
// endpoint GET /api/v1/autopilot/goals (RFC-204).
func Example_restAutopilotListGoals() {
	ts := newRestAutopilotServer()
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := restGet(ctx, ts.URL+"/api/v1/autopilot/goals")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	goals, _ := result["goals"].([]interface{})
	for _, g := range goals {
		if m, ok := g.(map[string]interface{}); ok {
			fmt.Printf("  - %s: %s (%s)\n", m["id"], m["text"], m["status"])
		}
	}
	// Output:
	//   - goal-1: Refactor auth module (in_progress)
	//   - goal-2: Add API tests (pending)
}

// Example_restAutopilotSubmitGoal submits a new goal via the HTTP REST endpoint
// POST /api/v1/autopilot/goals (RFC-204). Returns the created goal with status.
func Example_restAutopilotSubmitGoal() {
	ts := newRestAutopilotServer()
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, _, err := restPost(ctx, ts.URL+"/api/v1/autopilot/goals", map[string]interface{}{
		"text":     "Implement user authentication",
		"priority": 5,
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Goal ID: %v\n", result["id"])
	fmt.Printf("Text: %v\n", result["text"])
	fmt.Printf("Status: %v\n", result["status"])
	// Output:
	// Goal ID: goal-new
	// Text: Implement user authentication
	// Status: submitted
}

// Example_restAutopilotApproveGoal approves a goal via the HTTP REST endpoint
// POST /api/v1/autopilot/goals/{id}/approve (RFC-204). Returns 204 No Content.
func Example_restAutopilotApproveGoal() {
	ts := newRestAutopilotServer()
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, statusCode, err := restPost(ctx, ts.URL+"/api/v1/autopilot/goals/goal-1/approve", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Approve status: %d\n", statusCode)
	// Output:
	// Approve status: 204
}

// Example_restAutopilotRejectGoal rejects a goal via the HTTP REST endpoint
// POST /api/v1/autopilot/goals/{id}/reject (RFC-204). Returns 204 No Content.
func Example_restAutopilotRejectGoal() {
	ts := newRestAutopilotServer()
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, statusCode, err := restPost(ctx, ts.URL+"/api/v1/autopilot/goals/goal-1/reject", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Reject status: %d\n", statusCode)
	// Output:
	// Reject status: 204
}

// Example_restAutopilotWake transitions autopilot to wake mode via the HTTP
// REST endpoint POST /api/v1/autopilot/wake (RFC-204).
func Example_restAutopilotWake() {
	ts := newRestAutopilotServer()
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, _, err := restPost(ctx, ts.URL+"/api/v1/autopilot/wake", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Mode: %v\n", result["mode"])
	fmt.Printf("Status: %v\n", result["status"])
	// Output:
	// Mode: wake
	// Status: transitioned
}

// Example_restAutopilotDream transitions autopilot to dream mode via the HTTP
// REST endpoint POST /api/v1/autopilot/dream (RFC-204).
func Example_restAutopilotDream() {
	ts := newRestAutopilotServer()
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, _, err := restPost(ctx, ts.URL+"/api/v1/autopilot/dream", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Mode: %v\n", result["mode"])
	fmt.Printf("Status: %v\n", result["status"])
	// Output:
	// Mode: dream
	// Status: transitioned
}

// Example_restAutopilotFullFlow demonstrates a complete autopilot REST workflow:
// check status -> list goals -> submit a goal -> approve -> wake -> dream.
func Example_restAutopilotFullFlow() {
	ts := newRestAutopilotServer()
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Check autopilot status.
	status, err := restGet(ctx, ts.URL+"/api/v1/autopilot/status")
	if err != nil {
		fmt.Printf("Status error: %v\n", err)
		return
	}
	fmt.Printf("Status: mode=%v active_goals=%v\n", status["mode"], status["active_goals"])

	// 2. List existing goals.
	list, err := restGet(ctx, ts.URL+"/api/v1/autopilot/goals")
	if err != nil {
		fmt.Printf("List error: %v\n", err)
		return
	}
	fmt.Printf("Goals: %v\n", list["total"])

	// 3. Submit a new goal.
	newGoal, _, err := restPost(ctx, ts.URL+"/api/v1/autopilot/goals", map[string]interface{}{
		"text": "Deploy to production",
	})
	if err != nil {
		fmt.Printf("Submit error: %v\n", err)
		return
	}
	fmt.Printf("Submitted: %v\n", newGoal["id"])

	// 4. Approve a goal.
	_, approveCode, err := restPost(ctx, ts.URL+"/api/v1/autopilot/goals/goal-1/approve", nil)
	if err != nil {
		fmt.Printf("Approve error: %v\n", err)
		return
	}
	fmt.Printf("Approved: %d\n", approveCode)

	// 5. Transition to wake mode.
	wake, _, err := restPost(ctx, ts.URL+"/api/v1/autopilot/wake", nil)
	if err != nil {
		fmt.Printf("Wake error: %v\n", err)
		return
	}
	fmt.Printf("Wake: %v\n", wake["mode"])

	// 6. Transition to dream mode.
	dream, _, err := restPost(ctx, ts.URL+"/api/v1/autopilot/dream", nil)
	if err != nil {
		fmt.Printf("Dream error: %v\n", err)
		return
	}
	fmt.Printf("Dream: %v\n", dream["mode"])
	// Output:
	// Status: mode=idle active_goals=2
	// Goals: 2
	// Submitted: goal-new
	// Approved: 204
	// Wake: wake
	// Dream: dream
}
