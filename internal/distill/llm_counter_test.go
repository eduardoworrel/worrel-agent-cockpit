package distill

import (
	"context"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

type countingCLI struct {
	calls int
	resp  string
}

func (c *countingCLI) RunHeadless(_ context.Context, _ string, _ adapter.HeadlessOpts) (string, error) {
	c.calls++
	return c.resp, nil
}

func TestLLMCallsCountedOnSweep(t *testing.T) {
	s := newTestStore(t)
	cli := &countingCLI{resp: "[]"}
	e := New(s, cli, bus.New())

	p, _ := s.CreateProject("App", "")
	sess, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "observed"})
	// Conteúdo substancial com sinal de erro → passa Fase 1 e aciona o LLM.
	_ = s.AppendTranscriptEvent(sess.ID, "user", "text", "rode o deploy de staging com todos os passos do procedimento padrao", 0, 0)
	_ = s.AppendTranscriptEvent(sess.ID, "assistant", "text", "erro: o build falhou; vamos repetir os passos e corrigir ate ficar verde", 0, 0)
	_ = s.EndSession(sess.ID)
	// session is pending (analyzed_at NULL) + passa screening → sweep will call LLM

	before := cli.calls
	_, err := e.Sweep(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cli.calls <= before {
		t.Fatal("esperava pelo menos 1 LLM call ao ter sessões pendentes")
	}
	if e.LLMCalls() == 0 {
		t.Fatal("LLMCalls() deve retornar > 0")
	}
}

func TestScreeningSkipsLLMWhenNoSignals(t *testing.T) {
	s := newTestStore(t)
	cli := &countingCLI{resp: "[]"}
	e := New(s, cli, bus.New())

	// Sem sessões pendentes → sweep não chama LLM
	before := cli.calls
	_, err := e.Sweep(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cli.calls != before {
		t.Fatalf("sweep sem sessões não deve chamar LLM, calls = %d", cli.calls-before)
	}
}

// TestScreeningSkipsLLMWhenCandidateFailsPhase1 prova o critério de aceitação 5:
// uma sessão trivial (saudação curta, sem erros nem volume) é REPROVADA pelo
// screening de Fase 1 e NÃO aciona nenhuma chamada de LLM — verificável pelo
// contador de invocações do adaptador headless e por Engine.LLMCalls().
func TestScreeningSkipsLLMWhenCandidateFailsPhase1(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sess, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "x", Mode: "observed"})
	// Conteúdo trivial: sem palavras de erro, curto, poucos eventos → não passa Fase 1.
	_ = s.AppendTranscriptEvent(sess.ID, "user", "text", "oi", 0, 0)
	_ = s.EndSession(sess.ID)

	cli := &countingCLI{resp: "[]"}
	e := New(s, cli, bus.New())

	res, err := e.Sweep(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cli.calls != 0 {
		t.Fatalf("LLM chamado %d vez(es) para candidato reprovado na Fase 1 (critério 5)", cli.calls)
	}
	if e.LLMCalls() != 0 {
		t.Fatalf("contador LLMCalls = %d, esperado 0", e.LLMCalls())
	}
	if res.ScreenedOut != 1 {
		t.Fatalf("ScreenedOut = %d, esperado 1", res.ScreenedOut)
	}
	// A sessão deve ter sido marcada como analisada (não re-processável).
	got, _ := s.GetSession(sess.ID)
	if got.AnalyzedAt == nil {
		t.Fatal("sessão reprovada deve ser marcada analyzed_at para não reprocessar")
	}
}
