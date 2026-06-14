package handoff

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// fakeAdapter implements adapter.Adapter for testing.
type fakeAdapter struct {
	headlessResult string
	headlessErr    error
	lastPrompt     string
}

func (f *fakeAdapter) ID() string                      { return "fake" }
func (f *fakeAdapter) Detect() (adapter.Installed, error) { return adapter.Installed{}, nil }
func (f *fakeAdapter) Capabilities() adapter.Caps        { return adapter.Caps{Headless: true} }
func (f *fakeAdapter) BuildInteractive(opts adapter.SpawnOpts) (adapter.CmdSpec, error) {
	return adapter.CmdSpec{}, errors.New("not implemented")
}
func (f *fakeAdapter) RunHeadless(ctx context.Context, prompt string, opts adapter.HeadlessOpts) (string, error) {
	f.lastPrompt = prompt
	return f.headlessResult, f.headlessErr
}
func (f *fakeAdapter) DiscoverSessions(since time.Time) ([]adapter.ExternalSession, error) {
	return nil, adapter.ErrNotSupported
}
func (f *fakeAdapter) ReadTranscript(ref adapter.SessionRef) ([]adapter.TranscriptEvent, error) {
	return nil, adapter.ErrNotSupported
}
func (f *fakeAdapter) ContextUsage(ref adapter.SessionRef) (used, limit int, ok bool) {
	return 0, 0, false
}

func TestAdapterSummarizerDelegatesPrompt(t *testing.T) {
	fa := &fakeAdapter{headlessResult: "resultado do handoff"}
	sum := NewAdapterSummarizer(fa)

	out, err := sum.Summarize(context.Background(), "meu prompt de handoff")
	if err != nil {
		t.Fatal(err)
	}
	if out != "resultado do handoff" {
		t.Fatalf("resultado inesperado: %q", out)
	}
	if !strings.Contains(fa.lastPrompt, "meu prompt de handoff") {
		t.Fatalf("prompt não repassado: %q", fa.lastPrompt)
	}
}

func TestAdapterSummarizerPropagatesError(t *testing.T) {
	fa := &fakeAdapter{headlessErr: errors.New("falha headless")}
	sum := NewAdapterSummarizer(fa)

	_, err := sum.Summarize(context.Background(), "prompt")
	if err == nil || !strings.Contains(err.Error(), "falha headless") {
		t.Fatalf("erro não propagado: %v", err)
	}
}

const cleanMarkdown = "## Estado atual\nok\n## O que foi feito\nx\n## Decisões\nd\n## Próxima ação\nn\n## Não repetir\nr\n## Arquivos relevantes\n- a.go"

// rawStreamJSON imita a saída real de `claude -p --output-format json`:
// array de eventos, com o texto final num evento {"type":"result"}.
func rawStreamJSON(result string) string {
	events := []map[string]any{
		{"type": "system", "subtype": "init", "session_id": "abc"},
		{"type": "assistant", "message": map[string]any{"role": "assistant"}},
		{"type": "result", "subtype": "success", "is_error": false, "result": result},
	}
	b, _ := json.Marshal(events)
	return string(b)
}

func TestAdapterSummarizerStoresCleanMarkdown(t *testing.T) {
	s := newStore(t)
	p, _ := s.CreateProject("App", "")
	sess, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	_ = s.AppendTranscriptEvent(sess.ID, "user", "text", "implemente login", 0, 0)

	fa := &fakeAdapter{headlessResult: rawStreamJSON(cleanMarkdown)}
	g := New(s, NewAdapterSummarizer(fa))

	out, err := g.GenerateSummary(context.Background(), sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "## Estado atual") {
		t.Fatalf("resumo não é markdown limpo: %q", out[:min(len(out), 80)])
	}
	got, _ := s.GetSession(sess.ID)
	if !strings.HasPrefix(got.Summary, "## Estado atual") {
		t.Fatalf("summary persistido não é markdown limpo: %q", got.Summary[:min(len(got.Summary), 80)])
	}
	if strings.Contains(got.Summary, `"type":"system"`) {
		t.Fatal("summary persistido contém o envelope JSON do CLI")
	}
}

func TestExtractResultText(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"array de eventos", rawStreamJSON("## Estado atual\nok"), "## Estado atual\nok"},
		{"objeto único", `{"type":"result","result":"## Estado atual\nok"}`, "## Estado atual\nok"},
		{"texto puro", "## Estado atual\nok", "## Estado atual\nok"},
		{"json inválido passa direto", "[broken", "[broken"},
	}
	for _, c := range cases {
		if got := extractResultText(c.in); got != c.want {
			t.Fatalf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
