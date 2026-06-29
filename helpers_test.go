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
		defer conn.Close()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var m map[string]interface{}
			json.Unmarshal(msg, &m)
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
	defer client.Close()

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
		defer conn.Close()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var m map[string]interface{}
			json.Unmarshal(msg, &m)
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
		defer conn.Close()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var m map[string]interface{}
			json.Unmarshal(msg, &m)
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
	defer client.Close()

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
		defer conn.Close()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var m map[string]interface{}
			json.Unmarshal(msg, &m)
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
	defer client.Close()

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
		defer conn.Close()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var m map[string]interface{}
			json.Unmarshal(msg, &m)
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
	defer client.Close()

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
		defer conn.Close()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var m map[string]interface{}
			json.Unmarshal(msg, &m)
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
	defer client.Close()

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
		defer conn.Close()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var m map[string]interface{}
			json.Unmarshal(msg, &m)
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
	defer client.Close()

	config, err := FetchConfigSection(ctx, client, "providers", 3*time.Second)
	if err != nil {
		t.Fatalf("FetchConfigSection: %v", err)
	}
	if config["model"] != "gpt-4" {
		t.Errorf("model: %v", config["model"])
	}
}
