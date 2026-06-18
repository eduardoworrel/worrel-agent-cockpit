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
