package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/engine"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/engine/example"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func TestEnginesListAndRun(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := st.CreateProject("App", "")
	sess, _ := st.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	_ = st.AppendTranscriptEvent(sess.ID, "user", "text", "oi", 0, 0)

	reg := engine.NewRegistry()
	reg.Register(example.Counter{})
	srv := New(Deps{Store: st, Engines: reg})

	// GET /api/engines
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/api/engines", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "example-counter") {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}

	// POST run
	body := strings.NewReader(`{"project_id":"` + p.ID + `","session_id":"` + sess.ID + `"}`)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/api/engines/example-counter/run", body))
	if rec.Code != 200 {
		t.Fatalf("run: %d %s", rec.Code, rec.Body.String())
	}
	sgs, _ := st.ListSuggestions("", "")
	if len(sgs) != 1 {
		t.Fatalf("esperava 1 sugestão, got %d", len(sgs))
	}

	// PUT config
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("PUT", "/api/engines/example-counter/config",
		strings.NewReader(`{"key":"__enabled","value":"true"}`)))
	if rec.Code != 200 {
		t.Fatalf("config: %d %s", rec.Code, rec.Body.String())
	}
	var listResp []map[string]any
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/api/engines", nil))
	_ = json.Unmarshal(rec.Body.Bytes(), &listResp)
	if cfg, _ := listResp[0]["config"].(map[string]any); cfg["__enabled"] != "true" {
		t.Fatalf("config não refletiu: %v", listResp)
	}

	_ = http.StatusOK
}

func TestEngineBacklog(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := st.CreateProject("App", "")
	_, _ = st.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper", Status: "ended"})
	reg := engine.NewRegistry()
	reg.Register(example.Counter{})
	srv := New(Deps{Store: st, Engines: reg})

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/api/engines/example-counter/backlog", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"unanalyzed":1`) {
		t.Fatalf("backlog: %d %s", rec.Code, rec.Body.String())
	}
}

func TestEnginesEnabledEndpoint(t *testing.T) {
	st, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	defer st.Close()
	_ = st.SetEngineConfig("summary", "__enabled", "true", "session:s1")

	srv := &Server{deps: Deps{Store: st}, mux: http.NewServeMux()}
	srv.routesEngines()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/engines/summary/enabled?session_id=s1&default=false", nil)
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if !body.Enabled {
		t.Fatalf("esperava enabled=true para a sessão s1")
	}
}

func TestEnginesSettingsEndpoint(t *testing.T) {
	st, _ := store.Open(t.TempDir() + "/t.db")
	defer st.Close()
	_ = st.SetEngineConfig("summary", "__enabled", "true", "")
	_ = st.SetEngineConfig("summary", "harness", "opencode", "")
	_ = st.SetEngineConfig("summary", "model", "anthropic/claude-sonnet-4-6", "")

	srv := &Server{deps: Deps{Store: st}, mux: http.NewServeMux()}
	srv.routesEngines()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/engines/summary/settings", nil)
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var body struct {
		Enabled bool   `json:"enabled"`
		Harness string `json:"harness"`
		Model   string `json:"model"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if !body.Enabled || body.Harness != "opencode" || body.Model == "" {
		t.Fatalf("settings resolvido errado: %+v", body)
	}
}
