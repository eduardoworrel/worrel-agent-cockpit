package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/apply"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/mirror"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/workspace"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/wrapper"
)

func newFreeSessionServer(t *testing.T) (*httptest.Server, *store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(dir + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	s.SetDataDir(dir)
	t.Cleanup(func() { s.Close() })
	m := mirror.New(t.TempDir())
	b := bus.New()
	reg := adapter.NewRegistry()
	reg.Register(fakeCat{})
	wm := wrapper.New(s, b)
	wsm := workspace.New(dir)
	srv := New(Deps{Store: s, Mirror: m, Bus: b, Applier: apply.New(s, m, b),
		Wrapper: wm, Workspace: wsm, Adapters: reg, Port: 7717})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, s, dir
}

func TestFreeSessionCreates201WithScratchWorkspaceDir(t *testing.T) {
	ts, _, dir := newFreeSessionServer(t)

	body, _ := json.Marshal(map[string]string{"adapter": "fake"})
	resp, err := ts.Client().Post(ts.URL+"/api/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var sess store.Session
	json.NewDecoder(resp.Body).Decode(&sess)
	if sess.ProjectID != "" {
		t.Fatalf("sessão livre não deve ter project_id; got %q", sess.ProjectID)
	}
	wantPrefix := dir + "/workspaces/_scratch-"
	if !strings.HasPrefix(sess.WorkspaceDir, wantPrefix) {
		t.Fatalf("workspace_dir = %q, want prefix %q", sess.WorkspaceDir, wantPrefix)
	}
}

func TestClassifySession200(t *testing.T) {
	ts, s, _ := newFreeSessionServer(t)

	// cria sessão livre
	body, _ := json.Marshal(map[string]string{"adapter": "fake"})
	resp, _ := ts.Client().Post(ts.URL+"/api/sessions", "application/json", bytes.NewReader(body))
	var sess store.Session
	json.NewDecoder(resp.Body).Decode(&sess)

	// cria projeto
	p, _ := s.CreateProject("Escopo", "")

	// classifica
	classBody, _ := json.Marshal(map[string]string{"project_id": p.ID})
	cr, err := ts.Client().Post(ts.URL+"/api/sessions/"+sess.ID+"/classify",
		"application/json", bytes.NewReader(classBody))
	if err != nil {
		t.Fatal(err)
	}
	if cr.StatusCode != 200 {
		t.Fatalf("classify status = %d", cr.StatusCode)
	}

	// verifica store
	got, _ := s.GetSession(sess.ID)
	if got.ProjectID != p.ID {
		t.Fatalf("project_id não atualizado: %q", got.ProjectID)
	}
}

func TestPromoteSession201(t *testing.T) {
	ts, s, _ := newFreeSessionServer(t)

	// cria sessão livre
	body, _ := json.Marshal(map[string]string{"adapter": "fake"})
	resp, _ := ts.Client().Post(ts.URL+"/api/sessions", "application/json", bytes.NewReader(body))
	var sess store.Session
	json.NewDecoder(resp.Body).Decode(&sess)

	// promove
	promBody, _ := json.Marshal(map[string]string{"name": "Escopo", "description": "d"})
	pr, err := ts.Client().Post(ts.URL+"/api/sessions/"+sess.ID+"/promote",
		"application/json", bytes.NewReader(promBody))
	if err != nil {
		t.Fatal(err)
	}
	if pr.StatusCode != 201 {
		t.Fatalf("promote status = %d", pr.StatusCode)
	}
	var proj store.Project
	json.NewDecoder(pr.Body).Decode(&proj)
	if proj.Name != "Escopo" {
		t.Fatalf("proj.Name = %q", proj.Name)
	}
	// sessão deve estar classificada
	got, _ := s.GetSession(sess.ID)
	if got.ProjectID != proj.ID {
		t.Fatalf("sessão não classificada no projeto: %q", got.ProjectID)
	}
}

func TestGetActiveSessions(t *testing.T) {
	ts, s, _ := newFreeSessionServer(t)

	// sessão wrapper ativa (livre)
	body, _ := json.Marshal(map[string]string{"adapter": "fake"})
	resp, _ := ts.Client().Post(ts.URL+"/api/sessions", "application/json", bytes.NewReader(body))
	var sess store.Session
	json.NewDecoder(resp.Body).Decode(&sess)

	// sessão observed não deve aparecer
	s.CreateSession(&store.Session{Adapter: "fake", Mode: "observed"}) //nolint

	// aguarda sessão ser marcada como ativa
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r, _ := ts.Client().Get(ts.URL + "/api/sessions/active")
		var list []*store.Session
		json.NewDecoder(r.Body).Decode(&list)
		if len(list) == 1 && list[0].ID == sess.ID {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	r, _ := ts.Client().Get(ts.URL + "/api/sessions/active")
	if r.StatusCode != 200 {
		t.Fatalf("status = %d", r.StatusCode)
	}
	var list []*store.Session
	json.NewDecoder(r.Body).Decode(&list)
	if len(list) != 1 || list[0].ID != sess.ID {
		t.Fatalf("active sessions = %+v, want [%s]", list, sess.ID)
	}
}

func TestBadAdapterReturns400(t *testing.T) {
	ts, _, _ := newFreeSessionServer(t)

	body, _ := json.Marshal(map[string]string{"adapter": "naoexiste"})
	resp, err := ts.Client().Post(ts.URL+"/api/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	var errBody map[string]string
	json.NewDecoder(resp.Body).Decode(&errBody)
	if !strings.Contains(errBody["error"], "naoexiste") {
		t.Fatalf("error message = %q", errBody["error"])
	}
}

// Ensure the /api/sessions route doesn't shadow /api/sessions/{id}/... routes.
func TestActiveSessionsRouteNotShadowed(t *testing.T) {
	ts, _, _ := newFreeSessionServer(t)
	resp, err := ts.Client().Get(ts.URL + "/api/sessions/active")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("GET /api/sessions/active status = %d", resp.StatusCode)
	}
}

func TestFreeSessionWithInvalidJSON(t *testing.T) {
	ts, _, _ := newFreeSessionServer(t)
	resp, err := ts.Client().Post(ts.URL+"/api/sessions", "application/json",
		bytes.NewReader([]byte("not-json")))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

// Verify handleCreateSession still works through spawnFor.
func TestCreateSessionViaProjectStillWorks(t *testing.T) {
	ts, s, _ := newFreeSessionServer(t)
	p, _ := s.CreateProject("App", "")

	body, _ := json.Marshal(map[string]string{"adapter": "fake"})
	resp, err := ts.Client().Post(ts.URL+"/api/projects/"+p.ID+"/sessions",
		"application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var sess store.Session
	json.NewDecoder(resp.Body).Decode(&sess)
	if sess.ProjectID != p.ID {
		t.Fatalf("project_id = %q, want %q", sess.ProjectID, p.ID)
	}
}

