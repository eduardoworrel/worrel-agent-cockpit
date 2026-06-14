package apply

import (
	"strings"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func TestApplyPipeline(t *testing.T) {
	a, s := setup(t)
	p, err := s.CreateProject("App", "")
	if err != nil {
		t.Fatal(err)
	}
	s1, _ := s.CreateSkill(p.ID, "Coletar", "# coletar")
	s2, _ := s.CreateSkill(p.ID, "Relatar", "# relatar")

	payload := `{"name":"Fluxo","steps":[` +
		`{"skill_id":"` + s1.ID + `","note":"passo 1"},` +
		`{"skill_id":"orfa"},` +
		`{"skill_id":"` + s2.ID + `","note":"passo 2"}]}`
	sg, err := s.CreateSuggestion(&store.Suggestion{ProjectID: p.ID, Type: "pipeline",
		Title: "Fluxo", Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Accept(sg.ID); err != nil {
		t.Fatal(err)
	}
	pipes, err := s.ListPipelines(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(pipes) != 1 {
		t.Fatalf("pipelines = %d, want 1", len(pipes))
	}
	if !strings.Contains(pipes[0].Content, "Coletar") || !strings.Contains(pipes[0].Content, "Relatar") {
		t.Fatalf("content sem as etapas válidas: %s", pipes[0].Content)
	}
	got, _ := s.GetSuggestion(sg.ID)
	if got.Status != "accepted" {
		t.Fatalf("status = %s, want accepted", got.Status)
	}
}
