package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/apply"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/mirror"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/workspace"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/wrapper"
)

// fakeAdapter spawna /bin/cat em vez de um CLI real.
type fakeCat struct{}

func (fakeCat) ID() string                   { return "fake" }
func (fakeCat) Detect() (adapter.Installed, error) { return adapter.Installed{Present: true}, nil }
func (fakeCat) Capabilities() adapter.Caps   { return adapter.Caps{} }
func (fakeCat) BuildInteractive(o adapter.SpawnOpts) (adapter.CmdSpec, error) {
	return adapter.CmdSpec{Path: "/bin/cat", Dir: o.WorkingDir}, nil
}
func (fakeCat) RunHeadless(_ context.Context, p string, _ adapter.HeadlessOpts) (string, error) {
	return p, nil
}
func (fakeCat) DiscoverSessions(_ time.Time) ([]adapter.ExternalSession, error) { return nil, nil }
func (fakeCat) ReadTranscript(adapter.SessionRef) ([]adapter.TranscriptEvent, error) {
	return nil, nil
}
func (fakeCat) ContextUsage(ref adapter.SessionRef) (used, limit int, ok bool) { return 0, 0, false }

func newSessionsServer(t *testing.T) (*httptest.Server, *store.Store, *wrapper.Manager) {
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
	return ts, s, wm
}

func TestAdaptersEndpoint(t *testing.T) {
	ts, _, _ := newSessionsServer(t)
	resp, _ := ts.Client().Get(ts.URL + "/api/adapters")
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var det []adapter.DetectedAdapter
	json.NewDecoder(resp.Body).Decode(&det)
	if len(det) != 1 || det[0].ID != "fake" {
		t.Fatalf("adapters %+v", det)
	}
}

func TestCreateSessionSpawnsAndKill(t *testing.T) {
	ts, st, wm := newSessionsServer(t)
	p, _ := st.CreateProject("App", "")

	body, _ := json.Marshal(map[string]string{"adapter": "fake"})
	resp, err := ts.Client().Post(ts.URL+"/api/projects/"+p.ID+"/sessions",
		"application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var sess store.Session
	json.NewDecoder(resp.Body).Decode(&sess)
	if sess.ID == "" || sess.Status != "active" {
		t.Fatalf("sessão %+v", sess)
	}
	// está rodando no wrapper
	deadline := time.Now().Add(2 * time.Second)
	for !wm.IsRunning(sess.ID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !wm.IsRunning(sess.ID) {
		t.Fatal("sessão não está rodando")
	}

	// kill
	kr, _ := ts.Client().Post(ts.URL+"/api/sessions/"+sess.ID+"/kill", "application/json", nil)
	if kr.StatusCode != 204 {
		t.Fatalf("kill status %d", kr.StatusCode)
	}
}

// TestKillOrphanSessionEndsIt: sessão wrapper "órfã" (active no banco, sem PTY
// vivo — caso pós-restart do servidor). O × deve encerrá-la mesmo assim (204),
// senão Wrapper.Kill falhava com 404 e a sessão seguia na faixa de ativas.
func TestKillOrphanSessionEndsIt(t *testing.T) {
	ts, st, _ := newSessionsServer(t)
	sess, err := st.CreateSession(&store.Session{Adapter: "fake", Mode: "wrapper", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	// nenhum PTY foi spawnado para esta sessão (órfã)
	kr, _ := ts.Client().Post(ts.URL+"/api/sessions/"+sess.ID+"/kill", "application/json", nil)
	if kr.StatusCode != 204 {
		t.Fatalf("kill de sessão órfã = %d, want 204", kr.StatusCode)
	}
	active, _ := st.ListActiveWrapperSessions()
	for _, a := range active {
		if a.ID == sess.ID {
			t.Fatal("sessão órfã ainda aparece como ativa após kill")
		}
	}
	got, _ := st.GetSession(sess.ID)
	if got.Status != "ended" {
		t.Fatalf("status após kill = %q, want ended", got.Status)
	}
}

func TestWSOriginCheck(t *testing.T) {
	ts, st, _ := newSessionsServer(t)
	p, _ := st.CreateProject("App", "")
	body, _ := json.Marshal(map[string]string{"adapter": "fake"})
	resp, _ := ts.Client().Post(ts.URL+"/api/projects/"+p.ID+"/sessions",
		"application/json", bytes.NewReader(body))
	var sess store.Session
	json.NewDecoder(resp.Body).Decode(&sess)

	base := "ws" + strings.TrimPrefix(ts.URL, "http")
	for _, path := range []string{"/api/sessions/" + sess.ID + "/term", "/api/events"} {
		// origem externa → upgrade rejeitado (4xx)
		evil := http.Header{"Origin": []string{"http://evil.example"}}
		c, hresp, err := websocket.DefaultDialer.Dial(base+path, evil)
		if err == nil {
			c.Close()
			t.Fatalf("%s: upgrade com Origin evil deveria falhar", path)
		}
		if hresp == nil || hresp.StatusCode < 400 || hresp.StatusCode >= 500 {
			t.Fatalf("%s: esperava 4xx, got %+v", path, hresp)
		}

		// origem localhost (qualquer porta) → aceito
		local := http.Header{"Origin": []string{"http://127.0.0.1:7717"}}
		c2, _, err := websocket.DefaultDialer.Dial(base+path, local)
		if err != nil {
			t.Fatalf("%s: upgrade com Origin 127.0.0.1 falhou: %v", path, err)
		}
		c2.Close()
	}
}

func TestTerminalWS(t *testing.T) {
	ts, st, _ := newSessionsServer(t)
	p, _ := st.CreateProject("App", "")
	body, _ := json.Marshal(map[string]string{"adapter": "fake"})
	resp, _ := ts.Client().Post(ts.URL+"/api/projects/"+p.ID+"/sessions",
		"application/json", bytes.NewReader(body))
	var sess store.Session
	json.NewDecoder(resp.Body).Decode(&sess)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/sessions/" + sess.ID + "/term"
	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// envia stdin -> /bin/cat ecoa
	stdin, _ := json.Marshal(map[string]any{"type": "stdin", "data": "hello\n"})
	c.WriteMessage(websocket.TextMessage, stdin)

	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	var acc strings.Builder
	for {
		_, msg, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v (acc=%q)", err, acc.String())
		}
		acc.Write(msg)
		if strings.Contains(acc.String(), "hello") {
			break
		}
	}
}

// fakeCtxCat: como fakeCat, mas com ContextUsage real (fixo).
type fakeCtxCat struct{ fakeCat }

func (fakeCtxCat) ID() string { return "fake-ctx" }
func (fakeCtxCat) ContextUsage(ref adapter.SessionRef) (used, limit int, ok bool) {
	return 4200, 200000, true
}

func TestCreateSessionTracksContext(t *testing.T) {
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
	reg.Register(fakeCtxCat{})
	wm := wrapper.New(s, b)
	wsm := workspace.New(dir)
	srv := New(Deps{Store: s, Mirror: m, Bus: b, Applier: apply.New(s, m, b),
		Wrapper: wm, Workspace: wsm, Adapters: reg, Port: 7717})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	p, _ := s.CreateProject("App", "")
	body, _ := json.Marshal(map[string]string{"adapter": "fake-ctx"})
	resp, err := ts.Client().Post(ts.URL+"/api/projects/"+p.ID+"/sessions",
		"application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var sess store.Session
	json.NewDecoder(resp.Body).Decode(&sess)

	// O tracker faz uma medição imediata no spawn; aguarda brevemente o store refletir.
	deadline := time.Now().Add(2 * time.Second)
	var got *store.Session
	for time.Now().Before(deadline) {
		got, _ = s.GetSession(sess.ID)
		if got != nil && got.ContextUsed > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got == nil || got.ContextUsed != 4200 || got.ContextLimit != 200000 {
		t.Fatalf("contexto não rastreado: %+v", got)
	}

	_ = wm.Kill(sess.ID)
}
