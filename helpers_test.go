package soothe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckDaemonStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			var m map[string]interface{}
			_ = json.Unmarshal(msg, &m)
			if isConnectionInit(m) {
				testSendHandshake(conn, m)
				continue
			}
			id, _ := m["id"].(string)
			testSendResponse(conn, id, map[string]interface{}{"running": true, "port_live": true, "active_loops": 5})
		}
	}))
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	resp, err := CheckDaemonStatus(ctx, client, 3*time.Second)
	if err != nil {
		t.Fatalf("CheckDaemonStatus: %v", err)
	}
	if resp["running"] != true {
		t.Errorf("running: %v", resp["running"])
	}
}

func TestIsDaemonLive_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			var m map[string]interface{}
			_ = json.Unmarshal(msg, &m)
			if isConnectionInit(m) {
				testSendHandshake(conn, m)
				continue
			}
			id, _ := m["id"].(string)
			testSendResponse(conn, id, map[string]interface{}{"running": true, "port_live": true, "active_loops": 0})
		}
	}))
	defer ts.Close()

	if !IsDaemonLive(wsURL(ts.URL), 3*time.Second) {
		t.Error("expected daemon to be live")
	}
}

func TestIsDaemonLive_Failure(t *testing.T) {
	if IsDaemonLive("ws://localhost:59999", 500*time.Millisecond) {
		t.Error("expected daemon to not be live on bad port")
	}
}

func TestRequestDaemonShutdown(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			var m map[string]interface{}
			_ = json.Unmarshal(msg, &m)
			if isConnectionInit(m) {
				testSendHandshake(conn, m)
				continue
			}
			id, _ := m["id"].(string)
			testSendResponse(conn, id, map[string]interface{}{"status": "acknowledged"})
		}
	}))
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	if err := RequestDaemonShutdown(ctx, client, 3*time.Second); err != nil {
		t.Fatalf("RequestDaemonShutdown: %v", err)
	}
}

func TestRequestDaemonShutdown_NotAcknowledged(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			var m map[string]interface{}
			_ = json.Unmarshal(msg, &m)
			if isConnectionInit(m) {
				testSendHandshake(conn, m)
				continue
			}
			id, _ := m["id"].(string)
			testSendResponse(conn, id, map[string]interface{}{"status": "denied"})
		}
	}))
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	err := RequestDaemonShutdown(ctx, client, 3*time.Second)
	if err == nil {
		t.Error("expected error for non-acknowledged shutdown")
	}
}

func TestFetchSkillsCatalog(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			var m map[string]interface{}
			_ = json.Unmarshal(msg, &m)
			if isConnectionInit(m) {
				testSendHandshake(conn, m)
				continue
			}
			id, _ := m["id"].(string)
			testSendResponse(conn, id, map[string]interface{}{"skills": []interface{}{
				map[string]interface{}{"name": "research", "description": "Research skill"},
				map[string]interface{}{"name": "browser", "description": "Browser skill"},
			}})
		}
	}))
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	skills, err := FetchSkillsCatalog(ctx, client, 3*time.Second)
	if err != nil {
		t.Fatalf("FetchSkillsCatalog: %v", err)
	}
	if len(skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(skills))
	}
	if skills[0]["name"] != "research" {
		t.Errorf("skill name: %v", skills[0]["name"])
	}
}

func TestFetchSkillsCatalog_Empty(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			var m map[string]interface{}
			_ = json.Unmarshal(msg, &m)
			if isConnectionInit(m) {
				testSendHandshake(conn, m)
				continue
			}
			id, _ := m["id"].(string)
			testSendResponse(conn, id, map[string]interface{}{})
		}
	}))
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	skills, err := FetchSkillsCatalog(ctx, client, 3*time.Second)
	if err != nil {
		t.Fatalf("FetchSkillsCatalog: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}

func TestFetchConfigSection(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			var m map[string]interface{}
			_ = json.Unmarshal(msg, &m)
			if isConnectionInit(m) {
				testSendHandshake(conn, m)
				continue
			}
			id, _ := m["id"].(string)
			params, _ := m["params"].(map[string]interface{})
			section, _ := params["section"].(string)
			testSendResponse(conn, id, map[string]interface{}{
				section: map[string]interface{}{"api_key": "sk-***", "model": "gpt-4"},
			})
		}
	}))
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	config, err := FetchConfigSection(ctx, client, "providers", 3*time.Second)
	if err != nil {
		t.Fatalf("FetchConfigSection: %v", err)
	}
	if config["model"] != "gpt-4" {
		t.Errorf("model: %v", config["model"])
	}
}

func TestRequestDaemonConfigReload(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			var m map[string]interface{}
			_ = json.Unmarshal(msg, &m)
			if isConnectionInit(m) {
				testSendHandshake(conn, m)
				continue
			}
			id, _ := m["id"].(string)
			testSendResponse(conn, id, map[string]interface{}{"status": "reloaded"})
		}
	}))
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	if err := RequestDaemonConfigReload(ctx, client, 3*time.Second); err != nil {
		t.Fatalf("RequestDaemonConfigReload: %v", err)
	}
}

func TestFetchLoopHistory(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			var m map[string]interface{}
			_ = json.Unmarshal(msg, &m)
			if isConnectionInit(m) {
				testSendHandshake(conn, m)
				continue
			}
			id, _ := m["id"].(string)
			testSendResponse(conn, id, map[string]interface{}{
				"loop_id": "loop-123",
				"checkpoints": []interface{}{
					map[string]interface{}{"thread_id": "thread-1", "step": 0},
					map[string]interface{}{"thread_id": "thread-2", "step": 1},
				},
			})
		}
	}))
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	resp, err := FetchLoopHistory(ctx, client, "loop-123", 3*time.Second)
	if err != nil {
		t.Fatalf("FetchLoopHistory: %v", err)
	}
	if resp["loop_id"] != "loop-123" {
		t.Errorf("loop_id: %v", resp["loop_id"])
	}
	checkpoints, ok := resp["checkpoints"].([]interface{})
	if !ok || len(checkpoints) != 2 {
		t.Errorf("expected 2 checkpoints, got: %v", resp["checkpoints"])
	}
}

func TestRequestAuth(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			var m map[string]interface{}
			_ = json.Unmarshal(msg, &m)
			if isConnectionInit(m) {
				testSendHandshake(conn, m)
				continue
			}
			params, _ := m["params"].(map[string]interface{})
			if params["access_key"] != "ak-123" {
				t.Errorf("access_key: %v", params["access_key"])
			}
			id, _ := m["id"].(string)
			testSendResponse(conn, id, map[string]interface{}{"status": "authenticated"})
		}
	}))
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	resp, err := RequestAuth(ctx, client, "ak-123", "sk-456", 3*time.Second)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if resp["status"] != "authenticated" {
		t.Errorf("status: %v", resp["status"])
	}
}

func TestRequestAuthRefresh(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			var m map[string]interface{}
			_ = json.Unmarshal(msg, &m)
			if isConnectionInit(m) {
				testSendHandshake(conn, m)
				continue
			}
			params, _ := m["params"].(map[string]interface{})
			if params["refresh_token"] != "rt-789" {
				t.Errorf("refresh_token: %v", params["refresh_token"])
			}
			id, _ := m["id"].(string)
			testSendResponse(conn, id, map[string]interface{}{"status": "refreshed"})
		}
	}))
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	resp, err := RequestAuthRefresh(ctx, client, "rt-789", 3*time.Second)
	if err != nil {
		t.Fatalf("RefreshAuthToken: %v", err)
	}
	if resp["status"] != "refreshed" {
		t.Errorf("status: %v", resp["status"])
	}
}

func TestCronAdd(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			var m map[string]interface{}
			_ = json.Unmarshal(msg, &m)
			if isConnectionInit(m) {
				testSendHandshake(conn, m)
				continue
			}
			params, _ := m["params"].(map[string]interface{})
			if params["text"] != "every day at 9am run backup" {
				t.Errorf("text: %v", params["text"])
			}
			id, _ := m["id"].(string)
			testSendResponse(conn, id, map[string]interface{}{
				"job_id": "cron-001",
				"status": "scheduled",
			})
		}
	}))
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	resp, err := client.CronAdd(ctx, "every day at 9am run backup", 0, 3*time.Second)
	if err != nil {
		t.Fatalf("CronAdd: %v", err)
	}
	if resp["job_id"] != "cron-001" {
		t.Errorf("job_id: %v", resp["job_id"])
	}
}

func TestJobGuidance_UsesContentField(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			var m map[string]interface{}
			_ = json.Unmarshal(msg, &m)
			if isConnectionInit(m) {
				testSendHandshake(conn, m)
				continue
			}
			params, _ := m["params"].(map[string]interface{})
			// Verify the canonical field name is "content", not "text".
			if _, ok := params["text"]; ok {
				t.Errorf("expected 'content' field, got 'text': %v", params)
			}
			if params["content"] != "focus on quality" {
				t.Errorf("content: %v", params["content"])
			}
			id, _ := m["id"].(string)
			testSendResponse(conn, id, map[string]interface{}{
				"job_id":   "job-001",
				"absorbed": true,
			})
		}
	}))
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	resp, err := client.JobGuidance(ctx, "job-001", "focus on quality", "", 3*time.Second)
	if err != nil {
		t.Fatalf("JobGuidance: %v", err)
	}
	if resp["absorbed"] != true {
		t.Errorf("absorbed: %v", resp["absorbed"])
	}
}

func TestSendLoopSubscribe_UsesSubscribeEnvelope(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			var m map[string]interface{}
			_ = json.Unmarshal(msg, &m)
			if isConnectionInit(m) {
				testSendHandshake(conn, m)
				continue
			}
			// Verify the envelope type is "subscribe", not "request".
			if m["type"] != "subscribe" {
				t.Errorf("expected type=subscribe, got: %v", m["type"])
			}
			if m["method"] != "loop_events" {
				t.Errorf("expected method=loop_events, got: %v", m["method"])
			}
			id, _ := m["id"].(string)
			// Send a subscription confirmation next frame.
			testSendNext(conn, id, map[string]interface{}{
				"event":   "subscribed",
				"loop_id": "loop-123",
				"success": true,
			})
		}
	}))
	defer ts.Close()

	client := NewClient(wsURL(ts.URL), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.SendLoopSubscribe(ctx, "loop-123", "", ""); err != nil {
		t.Fatalf("SendLoopSubscribe: %v", err)
	}
}
