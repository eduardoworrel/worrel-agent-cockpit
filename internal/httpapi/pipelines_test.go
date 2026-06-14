package httpapi

import (
	"net/http/httptest"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/apply"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/mirror"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// pipelineTestServer monta um Server com a rota de pipelines registrada
// explicitamente (a fiação em routes() é feita fora deste pacote).
func pipelineTestServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	m := mirror.New(t.TempDir())
	srv := New(Deps{Store: s, Mirror: m, Bus: bus.New(), Applier: apply.New(s, m, bus.New())})
	// routes() (via Handler) já registra routesPipelines(); não registrar de novo.
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, s
}

func TestPipelinesAPICreateAndList(t *testing.T) {
	ts, s := pipelineTestServer(t)
	p, err := s.CreateProject("App", "")
	if err != nil {
		t.Fatal(err)
	}
	s1, _ := s.CreateSkill(p.ID, "Coletar", "# coletar")
	s2, _ := s.CreateSkill(p.ID, "Relatar", "# relatar")

	body := map[string]any{
		"name": "Fluxo",
		"steps": []map[string]string{
			{"skill_id": s1.ID, "note": "passo 1"},
			{"skill_id": s2.ID, "note": "passo 2"},
		},
	}
	var created store.Skill
	if code := postJSON(t, ts, "/api/projects/"+p.ID+"/pipelines", body, &created); code != 201 {
		t.Fatalf("POST status = %d, want 201", code)
	}
	if created.ID == "" {
		t.Fatal("pipeline criada sem id")
	}

	var list []store.Skill
	if code := getJSON(t, ts, "/api/projects/"+p.ID+"/pipelines", &list); code != 200 {
		t.Fatalf("GET status = %d, want 200", code)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list = %d, want 1 com id %s", len(list), created.ID)
	}
}
