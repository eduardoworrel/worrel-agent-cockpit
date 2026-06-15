package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/distill"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// headless que devolve uma sugestão de memória, simulando o LLM confirmando
// um aprendizado para a sessão.
type memoryHeadless struct{}

func (memoryHeadless) RunHeadless(_ context.Context, _ string, _ adapter.HeadlessOpts) (string, error) {
	return `[{"type":"add_memory","title":"pref","content":"usuário prefere X","evidence":"ocorreu 3x na sessão"}]`, nil
}

func TestDistillSessionEndpointCreatesSuggestion(t *testing.T) {
	s, _ := store.Open(t.TempDir() + "/t.db")
	t.Cleanup(func() { s.Close() })

	sess, err := s.CreateSession(&store.Session{Adapter: "claude-code", Mode: "wrapper"})
	if err != nil {
		t.Fatal(err)
	}
	// Transcrição substancial o suficiente para passar a fase 1 (screening).
	for i := 0; i < 10; i++ {
		_ = s.AppendTranscriptEvent(sess.ID, "user", "message",
			"como configuro o ambiente de desenvolvimento com várias etapas e erros recorrentes que precisam de cuidado", 0, 0)
		_ = s.AppendTranscriptEvent(sess.ID, "assistant", "message",
			"siga estes passos detalhados para resolver o erro de configuração do ambiente", 0, 0)
	}

	eng := distill.New(s, memoryHeadless{}, bus.New())
	srv := New(Deps{Store: s, Distiller: eng})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := ts.Client().Post(ts.URL+"/api/sessions/"+sess.ID+"/distill", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var res distill.Result
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatal(err)
	}
	if res.Created != 1 {
		t.Fatalf("esperava 1 sugestão criada, veio %d (screened_out=%d)", res.Created, res.ScreenedOut)
	}

	// A sugestão deve estar pendente com origem session.ended.
	pending, _ := s.ListSuggestions("", "pending")
	if len(pending) != 1 {
		t.Fatalf("esperava 1 sugestão pendente, veio %d", len(pending))
	}
	if pending[0].Origin != "session.ended" {
		t.Fatalf("origem = %q, esperava session.ended", pending[0].Origin)
	}
}

func TestDistillSessionUnavailableWhenNoEngine(t *testing.T) {
	s, _ := store.Open(t.TempDir() + "/t.db")
	t.Cleanup(func() { s.Close() })
	srv := New(Deps{Store: s}) // sem Distiller
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	resp, err := ts.Client().Post(ts.URL+"/api/sessions/whatever/distill", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, esperava 503", resp.StatusCode)
	}
}
