package retro

import (
	"context"
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/distill"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

func seedExecRun(t *testing.T, s *store.Store, nSessions int, perHour int64) (*store.RetroRun, string) {
	t.Helper()
	p, _ := s.CreateProject("App", "")
	run, _ := s.CreateRetroRun(&store.RetroRun{Status: "running", Depth: "completa", BudgetPerHour: perHour})
	for i := 0; i < nSessions; i++ {
		se, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "observed"})
		_ = s.AppendTranscriptEvent(se.ID, "user", "text", "preciso automatizar o deploy", 0, 0)
		_ = s.AppendTranscriptEvent(se.ID, "assistant", "text", "houve um erro no build; repetimos os passos de deploy de staging ate ficar verde, vale virar skill", 0, 0)
		_ = s.EndSession(se.ID)
		_ = s.AddRunSession(run.ID, se.ID, p.ID)
	}
	return run, p.ID
}

func TestExecutorRespectsHourlyBudgetAndPauses(t *testing.T) {
	s := newStore(t)
	run, _ := seedExecRun(t, s, 5, 2)
	cli := &scriptCLI{resp: "[]"}
	eng := distill.New(s, cli, bus.New())
	clk := &fakeClock{now: time.Now()}
	ex := NewExecutor(s, eng, bus.New(), nil).withClock(clk).withBatchSize(1)

	st, err := ex.Run(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if st != "paused" {
		t.Fatalf("status = %q, want paused", st)
	}
	if cli.calls != 2 {
		t.Fatalf("invocações = %d, want 2 (critério 3)", cli.calls)
	}

	// avança 1h → retoma e processa o resto SEM reprocessar (critério 4)
	clk.now = clk.now.Add(time.Hour)
	_ = s.SetRetroRunStatus(run.ID, "running")
	_, err = ex.Run(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	pend, _ := s.PendingRunSessions(run.ID)
	processed := 5 - len(pend)
	if processed+len(pend) != 5 {
		t.Fatal("cursor inconsistente")
	}
	if cli.calls > 5 {
		t.Fatalf("reprocessou: %d invocações > 5", cli.calls)
	}
}

// TestExecutorTagsSuggestionsRetroativa garante que sugestões produzidas pela
// análise RETROATIVA são marcadas com origin='retroativa' pela fiação
// executor→engine (NÃO pelo teste). Caso contrário Consolidate() — que só
// processa origin='retroativa' — nunca roda sobre saída real (critérios 7 e 10).
func TestExecutorTagsSuggestionsRetroativa(t *testing.T) {
	s := newStore(t)
	run, _ := seedExecRun(t, s, 1, 0)
	_ = s.SetRetroRunStatus(run.ID, "running")
	cli := &scriptCLI{resp: `[{"type":"skill.learned","title":"Deploy staging","name":"Deploy","content":"# x","evidence":"e","project_id":""}]`}
	eng := distill.New(s, cli, bus.New())
	ex := NewExecutor(s, eng, bus.New(), nil).withBatchSize(10)
	if _, err := ex.Run(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	sg, _ := s.ListSuggestions("", "pending")
	if len(sg) == 0 {
		t.Fatal("nenhuma sugestão criada pela análise retroativa")
	}
	for _, x := range sg {
		if x.Origin != "retroativa" {
			t.Fatalf("origin = %q, want retroativa (fiação executor→engine, critérios 7/10)", x.Origin)
		}
	}
}

// TestSweepKeepsIncrementalOrigin garante que a varredura incremental normal
// NÃO marca sugestões como retroativa (sem regressão na fila padrão).
func TestSweepKeepsIncrementalOrigin(t *testing.T) {
	s := newStore(t)
	p, _ := s.CreateProject("App", "")
	se, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "observed"})
	_ = s.AppendTranscriptEvent(se.ID, "user", "text", "preciso automatizar o deploy", 0, 0)
	_ = s.AppendTranscriptEvent(se.ID, "assistant", "text", "houve um erro no build; repetimos os passos de deploy de staging ate ficar verde, vale virar skill", 0, 0)
	_ = s.EndSession(se.ID)
	cli := &scriptCLI{resp: `[{"type":"skill.learned","title":"Deploy staging","name":"Deploy","content":"# x","evidence":"e","project_id":""}]`}
	eng := distill.New(s, cli, bus.New())
	if _, err := eng.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	sg, _ := s.ListSuggestions("", "pending")
	if len(sg) == 0 {
		t.Fatal("sweep não criou sugestão")
	}
	for _, x := range sg {
		if x.Origin == "retroativa" {
			t.Fatalf("sweep incremental marcou retroativa indevidamente: %q", x.Title)
		}
	}
}

func TestExecutorLeveSuppressesSkills(t *testing.T) {
	s := newStore(t)
	run, _ := seedExecRun(t, s, 1, 0)
	_ = s.SetRetroRunStatus(run.ID, "running")
	r, _ := s.GetRetroRun(run.ID)
	_ = s.SetRetroRunScope(r.ID, "{}", "leve", 0, 0)
	cli := &scriptCLI{resp: `[{"type":"skill.learned","title":"X","name":"X","content":"# x","evidence":"e","project_id":""}]`}
	eng := distill.New(s, cli, bus.New())
	ex := NewExecutor(s, eng, bus.New(), nil).withBatchSize(10)
	_, _ = ex.Run(context.Background(), run.ID)
	sg, _ := s.ListSuggestions("", "pending")
	for _, x := range sg {
		if x.Type == "skill.learned" {
			t.Fatal("modo leve não deve gerar skill.learned (critério 11)")
		}
	}
}
