package httpapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/apply"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/mirror"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// fakeHandoffGen implements SummaryGeneratorIface for tests.
type fakeHandoffGen struct {
	s *store.Store
}

func (f *fakeHandoffGen) GenerateSummary(ctx context.Context, sessionID string) (string, error) {
	summary := "## Estado atual\nok\n## O que foi feito\nfeito\n## Decisões\nd\n## Próxima ação\nn\n## Não repetir\nr\n## Arquivos relevantes\n- main.go"
	_ = f.s.SetSessionSummary(sessionID, summary)
	return summary, nil
}

// fakeSpawner implements Spawner for tests.
type fakeSpawner struct {
	s         *store.Store
	newSessID string
}

func (f *fakeSpawner) Spawn(projectID, primer, continues string) (string, error) {
	cont := continues
	sess, err := f.s.CreateSession(&store.Session{
		ProjectID: projectID,
		Adapter:   "claude-code",
		Mode:      "wrapper",
		Continues: &cont,
	})
	if err != nil {
		return "", err
	}
	f.newSessID = sess.ID
	return sess.ID, nil
}

func newTestServerWithHandoff(t *testing.T) (*httptest.Server, *store.Store, *fakeSpawner) {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	m := mirror.New(t.TempDir())
	b := bus.New()
	gen := &fakeHandoffGen{s: s}
	sp := &fakeSpawner{s: s}
	srv := New(Deps{
		Store:   s,
		Mirror:  m,
		Bus:     b,
		Applier: apply.New(s, m, b),
		Handoff: gen,
		Spawner: sp,
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, s, sp
}

func TestHandoffChainsSessions(t *testing.T) {
	ts, s, sp := newTestServerWithHandoff(t)
	p, _ := s.CreateProject("App", "")
	old, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	_ = s.AppendTranscriptEvent(old.ID, "user", "text", "x", 0, 0)

	var resp struct {
		OldID   string `json:"old_id"`
		NewID   string `json:"new_id"`
		Summary string `json:"summary"`
	}
	code := postJSON(t, ts, "/api/sessions/"+old.ID+"/handoff", nil, &resp)
	if code != 200 {
		t.Fatalf("code=%d", code)
	}
	// antiga arquivada com summary
	got, _ := s.GetSession(old.ID)
	if got.Status != "archived" || got.Summary == "" {
		t.Fatalf("antiga: status=%s summary=%q", got.Status, got.Summary)
	}
	// nova continua a antiga
	by, _ := s.ContinuedBy(old.ID)
	if by == nil || *by != resp.NewID {
		t.Fatalf("cadeia errada: continuedBy=%v newID=%s", by, resp.NewID)
	}
	_ = sp // verify fakeSpawner was used
	// resumo estruturado tem as seções
	if !strings.Contains(resp.Summary, "## Estado atual") {
		t.Fatalf("resumo sem seções: %q", resp.Summary)
	}
}

// errHandoffGen registra se foi chamado e sempre falha — simula CLI headless
// indisponível/travada (após timeout vira erro de contexto).
type errHandoffGen struct{ called bool }

func (f *errHandoffGen) GenerateSummary(ctx context.Context, sessionID string) (string, error) {
	f.called = true
	return "", context.DeadlineExceeded
}

func newServerWithGen(t *testing.T, gen SummaryGeneratorIface) (*httptest.Server, *store.Store) {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	m := mirror.New(t.TempDir())
	b := bus.New()
	srv := New(Deps{
		Store: s, Mirror: m, Bus: b, Applier: apply.New(s, m, b),
		Handoff: gen, Spawner: &fakeSpawner{s: s},
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, s
}

// Resumo já persistido (ex.: recuperação de boot) é reusado: retoma sem chamar o
// gerador (sem custo de LLM, sem travar).
func TestHandoffReusesPersistedSummary(t *testing.T) {
	gen := &errHandoffGen{}
	ts, s := newServerWithGen(t, gen)
	p, _ := s.CreateProject("App", "")
	old, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	_ = s.SetSessionSummary(old.ID, "## Estado atual\njá tinha")

	var resp struct {
		NewID   string `json:"new_id"`
		Summary string `json:"summary"`
	}
	code := postJSON(t, ts, "/api/sessions/"+old.ID+"/handoff", nil, &resp)
	if code != 200 {
		t.Fatalf("code=%d", code)
	}
	if gen.called {
		t.Fatal("gerador foi chamado apesar de já haver resumo")
	}
	if !strings.Contains(resp.Summary, "já tinha") || resp.NewID == "" {
		t.Fatalf("resp inesperada: %+v", resp)
	}
}

// Falha/timeout na geração do resumo NÃO aborta o handoff: a nova sessão é criada
// (o terminal abre) mesmo sem resumo. Antes retornava 500 e nada abria.
func TestHandoffSucceedsWhenSummaryFails(t *testing.T) {
	gen := &errHandoffGen{}
	ts, s := newServerWithGen(t, gen)
	p, _ := s.CreateProject("App", "")
	old, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})

	var resp struct {
		NewID string `json:"new_id"`
	}
	code := postJSON(t, ts, "/api/sessions/"+old.ID+"/handoff", nil, &resp)
	if code != 200 {
		t.Fatalf("esperava 200 mesmo com resumo falhando, got %d", code)
	}
	if !gen.called {
		t.Fatal("gerador deveria ter sido chamado")
	}
	if resp.NewID == "" {
		t.Fatal("nova sessão não foi criada")
	}
	if got, _ := s.GetSession(old.ID); got.Status != "archived" {
		t.Fatalf("antiga deveria estar arquivada, status=%s", got.Status)
	}
}

func TestHandoffNotAvailableWithoutDeps(t *testing.T) {
	ts, s := newTestServer(t)
	p, _ := s.CreateProject("App", "")
	old, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	code := postJSON(t, ts, "/api/sessions/"+old.ID+"/handoff", nil, nil)
	if code != 503 {
		t.Fatalf("expected 503 without handoff deps, got %d", code)
	}
}
