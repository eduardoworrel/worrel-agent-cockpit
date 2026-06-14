package retro

import (
	"context"
	"strings"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// seedProjectWithSuggestions cria um projeto com sessões na run e N sugestões
// destiladas (skill/memória) pendentes, simulando o estado pós-destilação.
func seedSynthProject(t *testing.T, s *store.Store, runID string, titles []string) string {
	t.Helper()
	p, _ := s.CreateProject("Proj", "")
	sess, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "observed"})
	_ = s.EndSession(sess.ID)
	_ = s.AddRunSession(runID, sess.ID, p.ID)
	for _, ti := range titles {
		_, _ = s.CreateSuggestion(&store.Suggestion{
			ProjectID: p.ID, Type: "skill.learned", Title: ti,
			Payload: `{"content":"# ` + ti + `"}`, Origin: "retroativa",
		})
	}
	return p.ID
}

func TestSynthesizeProposesWorkflow(t *testing.T) {
	s := newStore(t)
	run, _ := s.CreateRetroRun(&store.RetroRun{Status: "done", Depth: "completa"})
	pid := seedSynthProject(t, s, run.ID, []string{"Etapa A", "Etapa B", "Etapa C"})

	// O LLM reconhece as 3 etapas como um workflow e devolve UMA skill unificada.
	cli := &scriptCLI{resp: `[{"type":"skill.learned","title":"Workflow A→B→C","name":"Workflow A→B→C",` +
		`"content":"# 1. A\n2. B\n3. C","evidence":"unifica: Etapa A; Etapa B; Etapa C","project_id":"` + pid + `"}]`}
	sy := NewSynthesizer(s, cli, bus.New())
	if err := sy.Synthesize(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}

	pend, _ := s.ListSuggestions(pid, "pending")
	var wf *store.Suggestion
	for _, sg := range pend {
		if sg.Title == "Workflow A→B→C" {
			wf = sg
		}
	}
	if wf == nil {
		t.Fatalf("skill de workflow unificada não foi criada; sugestões=%d", len(pend))
	}
	if !strings.Contains(wf.Evidence, "unifica") {
		t.Fatalf("evidence deveria citar os itens unificados: %q", wf.Evidence)
	}
	if cli.calls != 1 {
		t.Fatalf("esperava 1 chamada de síntese, veio %d", cli.calls)
	}
}

func TestSynthesizeSkipsSmallProjects(t *testing.T) {
	s := newStore(t)
	run, _ := s.CreateRetroRun(&store.RetroRun{Status: "done", Depth: "completa"})
	// Só 1 skill (< 2 etapas) → não há workflow a costurar, nem chama o LLM.
	seedSynthProject(t, s, run.ID, []string{"Etapa única"})

	cli := &scriptCLI{resp: `[]`}
	sy := NewSynthesizer(s, cli, bus.New())
	if err := sy.Synthesize(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	if cli.calls != 0 {
		t.Fatalf("projeto pequeno não deveria acionar síntese, veio %d chamadas", cli.calls)
	}
}

func TestSynthesizeIsIdempotentByTitle(t *testing.T) {
	s := newStore(t)
	run, _ := s.CreateRetroRun(&store.RetroRun{Status: "done", Depth: "completa"})
	pid := seedSynthProject(t, s, run.ID, []string{"Etapa A", "Etapa B", "Etapa C"})

	// O LLM "propõe" um workflow cujo título já existe entre as pendentes → não duplica.
	cli := &scriptCLI{resp: `[{"type":"skill.learned","title":"Etapa A","name":"Etapa A",` +
		`"content":"# dup","evidence":"unifica: Etapa A","project_id":"` + pid + `"}]`}
	sy := NewSynthesizer(s, cli, bus.New())
	if err := sy.Synthesize(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	pend, _ := s.ListSuggestions(pid, "pending")
	n := 0
	for _, sg := range pend {
		if sg.Title == "Etapa A" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("título já existente foi duplicado (n=%d)", n)
	}
}
