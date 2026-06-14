package retro

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/distill"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func TestConsolidateCountsOccurrences(t *testing.T) {
	s := newStore(t)
	p, _ := s.CreateProject("App", "")
	run, _ := s.CreateRetroRun(&store.RetroRun{Status: "running", Depth: "completa"})
	// 3 sessões com a MESMA tarefa recorrente. A análise retroativa real produz,
	// num único lote, 3 candidatos skill.learned de títulos similares — todos
	// inseridos com origin='retroativa' pela fiação executor→engine (NÃO
	// fabricados aqui). Consolidate os funde contando occurrences=3 (critério 7).
	for i := 0; i < 3; i++ {
		se, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "observed"})
		_ = s.AppendTranscriptEvent(se.ID, "user", "text", "preciso automatizar o deploy", 0, 0)
		_ = s.AppendTranscriptEvent(se.ID, "assistant", "text", "erro no build; repetimos o deploy de staging ate ficar verde", 0, 0)
		_ = s.EndSession(se.ID)
		_ = s.AddRunSession(run.ID, se.ID, p.ID)
	}
	cli := &scriptCLI{resp: `[` +
		`{"type":"skill.learned","title":"Como fazer deploy no staging","name":"DeployA","content":"# x","evidence":"ev1","project_id":"` + p.ID + `"},` +
		`{"type":"skill.learned","title":"Como fazer deploy no staging","name":"DeployB","content":"# x","evidence":"ev2","project_id":"` + p.ID + `"},` +
		`{"type":"skill.learned","title":"Como fazer deploy no staging","name":"DeployC","content":"# x","evidence":"ev3","project_id":"` + p.ID + `"}]`}
	eng := distill.New(s, cli, bus.New())
	ex := NewExecutor(s, eng, bus.New(), nil).withBatchSize(10)
	if _, err := ex.Run(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	// pré-condição: a fiação marcou origin='retroativa' (senão Consolidate ignora)
	for _, sg := range mustList(t, s, p.ID) {
		if sg.Origin != "retroativa" {
			t.Fatalf("origin = %q, want retroativa (fiação executor→engine)", sg.Origin)
		}
	}
	con := NewConsolidator(s)
	if err := con.Consolidate(run.ID); err != nil {
		t.Fatal(err)
	}
	pend, _ := s.ListSuggestions(p.ID, "pending")
	if len(pend) != 1 {
		t.Fatalf("após consolidar = %d, want 1", len(pend))
	}
	var pl map[string]any
	_ = json.Unmarshal([]byte(pend[0].Payload), &pl)
	if occ, _ := pl["occurrences"].(float64); int(occ) != 3 {
		t.Fatalf("occurrences = %v, want 3 (critério 7)", pl["occurrences"])
	}

	// BatchView agrupa projeto -> tipo
	view, _ := con.BatchView(run.ID)
	if len(view) != 1 || len(view[0].Groups) != 1 || view[0].Groups[0].Type != "skill.learned" {
		t.Fatalf("batch view inesperada: %+v", view)
	}
	if len(view[0].Groups[0].Items) != 1 {
		t.Fatalf("itens no grupo = %d, want 1", len(view[0].Groups[0].Items))
	}
}

// retroSuggestion insere uma sugestão de origem retroativa, vinculada à run,
// para os testes de consolidação semântica.
func retroSuggestion(t *testing.T, s *store.Store, runID, pid, typ, title, content, evidence string) *store.Suggestion {
	t.Helper()
	se, _ := s.CreateSession(&store.Session{ProjectID: pid, Adapter: "claude-code", Mode: "observed"})
	_ = s.AddRunSession(runID, se.ID, pid)
	payload, _ := json.Marshal(map[string]any{"content": content})
	sg, err := s.CreateSuggestion(&store.Suggestion{
		ProjectID: pid, SessionID: &se.ID, Type: typ, Title: title,
		Payload: string(payload), Evidence: evidence, Origin: "retroativa",
	})
	if err != nil {
		t.Fatal(err)
	}
	return sg
}

// Mesmo projeto/tipo, TÍTULOS diferentes mas mesma lição no corpo/evidência:
// a consolidação semântica funde em 1, preservando ambas as evidências.
func TestConsolidateSemanticDifferentTitles(t *testing.T) {
	s := newStore(t)
	p, _ := s.CreateProject("App", "")
	run, _ := s.CreateRetroRun(&store.RetroRun{Status: "running", Depth: "completa"})
	retroSuggestion(t, s, run.ID, p.ID, "skill.learned",
		"Servidor local servindo versão antiga",
		"o servidor local continuava servindo a versão antiga do bundle por causa do cache; o deploy real não tinha sido aplicado",
		"evid-A")
	retroSuggestion(t, s, run.ID, p.ID, "skill.learned",
		"Verificar deploy real: bundle vs cache",
		"sempre verificar se o deploy real do bundle foi aplicado e não é apenas cache antigo do servidor local servindo a versão antiga",
		"evid-B")

	con := NewConsolidator(s)
	if err := con.Consolidate(run.ID); err != nil {
		t.Fatal(err)
	}
	pend, _ := s.ListSuggestions(p.ID, "pending")
	if len(pend) != 1 {
		t.Fatalf("após consolidar semântica = %d, want 1", len(pend))
	}
	var pl map[string]any
	_ = json.Unmarshal([]byte(pend[0].Payload), &pl)
	if occ, _ := pl["occurrences"].(float64); int(occ) != 2 {
		t.Fatalf("occurrences = %v, want 2", pl["occurrences"])
	}
	if !strings.Contains(pend[0].Evidence, "evid-A") || !strings.Contains(pend[0].Evidence, "evid-B") {
		t.Fatalf("evidências não preservadas: %q", pend[0].Evidence)
	}
}

// Controle: duas lições genuinamente distintas (mesmo projeto/tipo) NÃO são fundidas.
func TestConsolidateKeepsDistinct(t *testing.T) {
	s := newStore(t)
	p, _ := s.CreateProject("App", "")
	run, _ := s.CreateRetroRun(&store.RetroRun{Status: "running", Depth: "completa"})
	retroSuggestion(t, s, run.ID, p.ID, "skill.learned",
		"Configurar pipeline de deploy no staging",
		"como configurar o pipeline de deploy contínuo para o ambiente de staging usando o runner",
		"evid-deploy")
	retroSuggestion(t, s, run.ID, p.ID, "skill.learned",
		"Otimizar consultas SQL lentas no relatório",
		"adicionar índices e reescrever joins para acelerar as consultas do relatório financeiro mensal",
		"evid-sql")

	con := NewConsolidator(s)
	if err := con.Consolidate(run.ID); err != nil {
		t.Fatal(err)
	}
	pend, _ := s.ListSuggestions(p.ID, "pending")
	if len(pend) != 2 {
		t.Fatalf("lições distintas fundidas indevidamente = %d, want 2", len(pend))
	}
}

func mustList(t *testing.T, s *store.Store, pid string) []*store.Suggestion {
	t.Helper()
	sg, err := s.ListSuggestions(pid, "pending")
	if err != nil {
		t.Fatal(err)
	}
	return sg
}
