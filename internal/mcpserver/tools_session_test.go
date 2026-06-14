package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func TestGetSessionSummaryWithToken(t *testing.T) {
	svc, s, _ := setup(t)
	p, _ := s.CreateProject("App", "")
	tok := "sestoken1"
	sess, _ := s.CreateSession(&store.Session{
		ProjectID: p.ID,
		Adapter:   "claude-code",
		Mode:      "wrapper",
		MCPToken:  &tok,
	})
	s.AppendTranscriptEvent(sess.ID, "user", "message", "hello world content here", 0, 0)

	cs := connect(t, svc, tok)

	out := callText(t, cs, "get_session_summary", map[string]any{})
	if out == "" {
		t.Fatal("get_session_summary returned empty")
	}
	// Digest must include the actual transcript content.
	if !strings.Contains(out, "hello") {
		t.Fatalf("get_session_summary missing transcript content: %s", out)
	}
}

func TestGetSessionSummaryNoTokenNoID(t *testing.T) {
	svc, _, _ := setup(t)
	cs := connect(t, svc, "") // external, no token

	out := callText(t, cs, "get_session_summary", map[string]any{})
	if !strings.Contains(out, "session_id") {
		t.Fatalf("should require session_id: %s", out)
	}
}

func TestGetSessionSummaryExplicitID(t *testing.T) {
	svc, s, _ := setup(t)
	p, _ := s.CreateProject("App", "")
	sess, _ := s.CreateSession(&store.Session{
		ProjectID: p.ID,
		Adapter:   "claude-code",
		Mode:      "wrapper",
	})
	s.AppendTranscriptEvent(sess.ID, "assistant", "message", "some response here for test", 0, 0)

	cs := connect(t, svc, "") // external, no token

	out := callText(t, cs, "get_session_summary", map[string]any{"session_id": sess.ID})
	if !strings.Contains(out, "some response") && !strings.Contains(out, "Resumo") {
		t.Fatalf("get_session_summary with explicit id: %s", out)
	}
}

type fakeSummaryGen struct {
	summary string
	s       *store.Store
}

func (f *fakeSummaryGen) GenerateSummary(ctx context.Context, sessionID string) (string, error) {
	if f.s != nil {
		_ = f.s.SetSessionSummary(sessionID, f.summary)
	}
	return f.summary, nil
}

func TestGetSessionSummaryGeneratesWhenEmpty(t *testing.T) {
	svc, s, b := setup(t)
	p, _ := s.CreateProject("App", "")
	tok := "tok-gen-test"
	sess, _ := s.CreateSession(&store.Session{
		ProjectID: p.ID,
		Adapter:   "claude-code",
		Mode:      "wrapper",
		MCPToken:  &tok,
	})
	_ = s.AppendTranscriptEvent(sess.ID, "user", "text", "implemente algo", 0, 0)
	_ = b // use b to avoid unused

	structured := "## Estado atual\nok\n## O que foi feito\nfeito\n## Decisões\nd\n## Próxima ação\nn\n## Não repetir\nr\n## Arquivos relevantes\n- main.go"
	gen := &fakeSummaryGen{summary: structured, s: s}
	svc.WithSummaryGenerator(gen)

	cs := connect(t, svc, tok)
	out := callText(t, cs, "get_session_summary", map[string]any{})
	if !strings.Contains(out, "## Estado atual") {
		t.Fatalf("resumo estruturado não retornado: %q", out)
	}
	// Persistido
	got, _ := s.GetSession(sess.ID)
	if got.Summary == "" {
		t.Fatal("summary não foi persistido no store")
	}
}
