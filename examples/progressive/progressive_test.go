package progressive_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	soothe "github.com/mirasoth/soothe-client-go"
	"github.com/mirasoth/soothe-client-go/appkit"
)

type progressiveMock struct {
	URL      string
	server   *httptest.Server
	mu       sync.Mutex
	loopN    int
	jobN     int
	upgrader websocket.Upgrader
	pushTurn bool
}

func newProgressiveMock(t *testing.T) *progressiveMock {
	t.Helper()
	md := &progressiveMock{upgrader: websocket.Upgrader{}, pushTurn: true}
	md.server = httptest.NewServer(http.HandlerFunc(md.handle))
	md.URL = "ws" + strings.TrimPrefix(md.server.URL, "http")
	t.Cleanup(md.server.Close)
	return md
}

func (md *progressiveMock) nextLoop() string {
	md.mu.Lock()
	defer md.mu.Unlock()
	md.loopN++
	return fmt.Sprintf("loop-%d", md.loopN)
}

func (md *progressiveMock) nextJob() string {
	md.mu.Lock()
	defer md.mu.Unlock()
	md.jobN++
	return fmt.Sprintf("job-%d", md.jobN)
}

func (md *progressiveMock) write(conn *websocket.Conn, v interface{}) {
	b, _ := json.Marshal(v)
	_ = conn.WriteMessage(websocket.TextMessage, b)
}

func (md *progressiveMock) handle(w http.ResponseWriter, r *http.Request) {
	conn, err := md.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var m map[string]interface{}
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		typ, _ := m["type"].(string)
		method, _ := m["method"].(string)
		id, _ := m["id"].(string)
		params, _ := m["params"].(map[string]interface{})
		if params == nil {
			params = map[string]interface{}{}
		}
		switch {
		case typ == "connection_init":
			md.write(conn, map[string]interface{}{"proto": "1", "type": "status", "state": "idle"})
			md.write(conn, map[string]interface{}{
				"proto": "1", "type": "connection_ack",
				"result": map[string]interface{}{
					"protocol_version": "1", "readiness_state": "ready",
					"capabilities":          []string{"streaming", "batch", "heartbeat"},
					"heartbeat_interval_ms": 0,
				},
			})
		case typ == "request" && method == "loop_new":
			md.write(conn, map[string]interface{}{
				"proto": "1", "type": "response", "id": id,
				"result": map[string]interface{}{"loop_id": md.nextLoop(), "success": true},
			})
		case typ == "subscribe" && method == "loop_events":
			md.write(conn, map[string]interface{}{
				"proto": "1", "type": "next", "id": id,
				"payload": map[string]interface{}{"success": true, "loop_id": params["loop_id"]},
			})
		case typ == "notification" && method == "loop_input":
			if !md.pushTurn {
				continue
			}
			lid := params["loop_id"]
			md.write(conn, map[string]interface{}{"type": "status", "state": "running", "loop_id": lid})
			md.write(conn, map[string]interface{}{
				"type": "event", "mode": "messages", "loop_id": lid, "namespace": []interface{}{},
				"data": []interface{}{
					map[string]interface{}{"type": "ai", "content": "hi", "phase": "direct_model"},
					map[string]interface{}{},
				},
			})
			md.write(conn, map[string]interface{}{
				"type": "event", "mode": "custom", "loop_id": lid, "namespace": []interface{}{},
				"data": map[string]interface{}{"type": "soothe.stream.end", "scope": "turn"},
			})
		case typ == "request" && strings.HasPrefix(method, "job_"):
			result := map[string]interface{}{"ok": true}
			if method == "job_create" {
				result["job_id"] = md.nextJob()
			} else if jid, ok := params["job_id"].(string); ok {
				result["job_id"] = jid
			}
			md.write(conn, map[string]interface{}{
				"proto": "1", "type": "response", "id": id, "result": result,
			})
		case typ == "request":
			md.write(conn, map[string]interface{}{
				"proto": "1", "type": "response", "id": id, "result": map[string]interface{}{"ok": true},
			})
		case typ == "notification":
			// ignore
		}
	}
}

type memStore struct {
	loops map[string]string
}

func (s *memStore) GetSession(ctx context.Context, appKey appkit.AppKey) (*appkit.SessionEntry, error) {
	return nil, nil
}
func (s *memStore) CreateSession(ctx context.Context, workspaceID string, appKey appkit.AppKey, loopID, sessionType string) error {
	s.loops[appKey] = loopID
	return nil
}
func (s *memStore) UpdateLastUsed(ctx context.Context, appKey appkit.AppKey) error { return nil }
func (s *memStore) IncrementResetCount(ctx context.Context, appKey appkit.AppKey) error {
	return nil
}
func (s *memStore) GetLoopIDForSession(ctx context.Context, appKey appkit.AppKey) (string, bool, error) {
	id, ok := s.loops[appKey]
	return id, ok, nil
}
func (s *memStore) AppendMessage(ctx context.Context, appKey appkit.AppKey, message appkit.SessionMessage) error {
	return nil
}

func TestProgressive_01_Hello(t *testing.T) {
	md := newProgressiveMock(t)
	client := soothe.NewClient(md.URL, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := soothe.ConnectWithRetries(ctx, client, 5, 50*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	cfg := soothe.DefaultConfig()
	cfg.LoopStatusTimeout = 5 * time.Second
	cfg.SubscriptionTimeout = 5 * time.Second
	loopID, err := soothe.BootstrapLoopSession(ctx, client, "", "", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if loopID == "" {
		t.Fatal("empty loop id")
	}
}

func TestProgressive_02_StreamTurn(t *testing.T) {
	md := newProgressiveMock(t)
	cfg := soothe.DefaultConfig()
	cfg.LoopStatusTimeout = 5 * time.Second
	cfg.SubscriptionTimeout = 5 * time.Second
	cfg.DaemonReadyTimeout = 5 * time.Second
	session := appkit.NewDaemonSession(md.URL, &appkit.DaemonSessionOptions{
		Config: cfg, PostIdleDrain: 20 * time.Millisecond,
	})
	defer session.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := session.Connect(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := session.SendTurn(ctx, "hello", &appkit.SendTurnOptions{
		IntentHint: soothe.IntentHintTextCompletion,
	}); err != nil {
		t.Fatal(err)
	}
	chunks, errCh := session.IterTurnChunks(ctx, 5*time.Second)
	n := 0
	for range chunks {
		n++
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("expected chunks")
	}
}

func TestProgressive_03_TextCompletion(t *testing.T) {
	md := newProgressiveMock(t)
	cfg := soothe.DefaultConfig()
	cfg.DaemonReadyTimeout = 5 * time.Second
	session := appkit.NewDaemonSession(md.URL, &appkit.DaemonSessionOptions{Config: cfg})
	defer session.Close()
	ctx := context.Background()
	if _, err := session.Connect(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := session.SendTurn(ctx, "one word", &appkit.SendTurnOptions{
		IntentHint: soothe.IntentHintTextCompletion,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestProgressive_04_MultiTurn(t *testing.T) {
	md := newProgressiveMock(t)
	md.pushTurn = false
	cfg := soothe.DefaultConfig()
	cfg.DaemonReadyTimeout = 5 * time.Second
	session := appkit.NewDaemonSession(md.URL, &appkit.DaemonSessionOptions{Config: cfg})
	defer session.Close()
	ctx := context.Background()
	if _, err := session.Connect(ctx, ""); err != nil {
		t.Fatal(err)
	}
	for _, msg := range []string{"first", "second"} {
		if err := session.SendTurn(ctx, msg, &appkit.SendTurnOptions{
			IntentHint: soothe.IntentHintTextCompletion,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if session.LoopID() == "" {
		t.Fatal("expected loop id")
	}
}

func TestProgressive_05_PoolService(t *testing.T) {
	store := &memStore{loops: map[string]string{}}
	pool := appkit.NewConnectionPool("ws://127.0.0.1:9", store, &appkit.PoolConfig{
		PoolSize: 2, MaxIdleTime: time.Minute, HealthCheckInterval: time.Second,
	}, nil, nil)
	active, idle := pool.Stats()
	if active != 0 || idle != 2 {
		t.Fatalf("stats active=%d idle=%d", active, idle)
	}
}

func TestProgressive_06_Jobs(t *testing.T) {
	md := newProgressiveMock(t)
	cc := soothe.NewCommandClient(md.URL, 5*time.Second)
	ctx := context.Background()
	created, err := cc.JobCreate(ctx, "list files", "")
	if err != nil {
		t.Fatal(err)
	}
	jobID, _ := created["job_id"].(string)
	if jobID == "" {
		t.Fatalf("no job_id: %#v", created)
	}
	if _, err := cc.JobStatus(ctx, jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := cc.JobCancel(ctx, jobID); err != nil {
		t.Fatal(err)
	}
}
