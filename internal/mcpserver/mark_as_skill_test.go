package mcpserver

import (
	"strings"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func TestMarkAsSkillHelper(t *testing.T) {
	svc, s, _ := setup(t)
	p, _ := s.CreateProject("App", "")
	tok := "tok-mark-skill"
	sess, _ := s.CreateSession(&store.Session{
		ProjectID: p.ID,
		Adapter:   "claude-code",
		Mode:      "wrapper",
		MCPToken:  &tok,
	})

	cand, err := svc.markAsSkill(sess.ID, p.ID, "Meu Fluxo")
	if err != nil {
		t.Fatalf("markAsSkill error: %v", err)
	}
	if cand == nil {
		t.Fatal("markAsSkill returned nil candidate")
	}
	if cand.ExplicitMark != 1 {
		t.Fatalf("expected explicit_mark=1, got %d", cand.ExplicitMark)
	}
	if cand.Title != "Meu Fluxo" {
		t.Fatalf("expected title 'Meu Fluxo', got %q", cand.Title)
	}
}

func TestMarkAsSkillMCPTool(t *testing.T) {
	svc, s, _ := setup(t)
	p, _ := s.CreateProject("App", "")
	tok := "tok-mark-skill-mcp"
	_, _ = s.CreateSession(&store.Session{
		ProjectID: p.ID,
		Adapter:   "claude-code",
		Mode:      "wrapper",
		MCPToken:  &tok,
	})

	cs := connect(t, svc, tok)

	out := callText(t, cs, "mark_as_skill", map[string]any{"title": "Deploy Flow"})
	if strings.Contains(out, "erro") || strings.Contains(out, "error") {
		t.Fatalf("mark_as_skill returned error: %s", out)
	}
	if !strings.Contains(out, "candidato") && !strings.Contains(out, "marcado") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestMarkAsSkillNoProject(t *testing.T) {
	svc, _, _ := setup(t)
	cs := connect(t, svc, "") // no token = no project

	out := callText(t, cs, "mark_as_skill", map[string]any{"title": "something"})
	if !strings.Contains(out, "projeto") && !strings.Contains(out, "session") {
		t.Fatalf("expected error about missing project, got: %s", out)
	}
}
