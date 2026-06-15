package handoff

import (
	"context"
	"strings"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// fakeLive devolve eventos ao vivo para um ref dado (simula ler o .jsonl).
type fakeLive struct{ byRef map[string][]adapter.TranscriptEvent }

func (f *fakeLive) ReadTranscript(ref adapter.SessionRef) ([]adapter.TranscriptEvent, error) {
	return f.byRef[ref.ExternalRef], nil
}

type fakeSummarizer struct{ gotPrompt string }

func (f *fakeSummarizer) Summarize(ctx context.Context, prompt string) (string, error) {
	f.gotPrompt = prompt
	return "## Estado atual\nok\n## O que foi feito\nx\n## Decisões\nd\n## Próxima ação\nn\n## Não repetir\nr\n## Arquivos relevantes\n- a.go", nil
}

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestGenerateSummaryPersists(t *testing.T) {
	s := newStore(t)
	p, _ := s.CreateProject("App", "")
	sess, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	_ = s.AppendTranscriptEvent(sess.ID, "user", "text", "implemente login", 0, 0)
	_ = s.AppendTranscriptEvent(sess.ID, "assistant", "text", "feito, testes passam", 0, 0)

	fake := &fakeSummarizer{}
	g := New(s, fake)

	out, err := g.GenerateSummary(context.Background(), sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	// O prompt fixo cita as 6 seções obrigatórias.
	for _, sec := range []string{"Estado atual", "O que foi feito", "Decisões", "Próxima ação", "Não repetir", "Arquivos relevantes"} {
		if !strings.Contains(fake.gotPrompt, sec) {
			t.Fatalf("prompt não cita seção %q", sec)
		}
	}
	// O transcript normalizado entra no prompt.
	if !strings.Contains(fake.gotPrompt, "implemente login") {
		t.Fatal("transcript não foi incluído no prompt")
	}
	// Persistido em sessions.summary.
	got, _ := s.GetSession(sess.ID)
	if got.Summary != out || got.Summary == "" {
		t.Fatalf("summary não persistido: %q", got.Summary)
	}
}

// Sessão in-app não tem eventos em transcript_events; o handoff deve ler o
// transcript ao vivo (pelo external_ref == id) em vez de degradar para vazio.
func TestGenerateSummaryReadsLiveTranscriptForInApp(t *testing.T) {
	s := newStore(t)
	p, _ := s.CreateProject("App", "")
	sess, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	// CreateSession grava external_ref = id para sessões wrapper.
	if sess.ExternalRef == nil || *sess.ExternalRef != sess.ID {
		t.Fatalf("external_ref da sessão in-app deveria ser o id, got %v", sess.ExternalRef)
	}
	// Nenhum AppendTranscriptEvent: transcript_events está vazio (caso real).
	live := &fakeLive{byRef: map[string][]adapter.TranscriptEvent{
		sess.ID: {{Role: "user", Kind: "text", Content: "implemente login"}},
	}}
	fake := &fakeSummarizer{}
	g := New(s, fake).WithLiveReader(live)

	if _, err := g.GenerateSummary(context.Background(), sess.ID); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fake.gotPrompt, "implemente login") {
		t.Fatalf("transcript ao vivo não entrou no prompt: %q", fake.gotPrompt)
	}
	if strings.Contains(fake.gotPrompt, "sem eventos de transcript") {
		t.Fatal("caiu no fallback vazio mesmo com transcript ao vivo disponível")
	}
}

func TestGenerateSummaryEmptyTranscript(t *testing.T) {
	s := newStore(t)
	p, _ := s.CreateProject("App", "")
	sess, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	g := New(s, &fakeSummarizer{})
	if _, err := g.GenerateSummary(context.Background(), sess.ID); err != nil {
		t.Fatalf("transcript vazio deve gerar resumo (degrada): %v", err)
	}
}
