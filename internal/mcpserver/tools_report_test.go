package mcpserver

import (
	"strings"
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func TestReportToolsCreateSuggestions(t *testing.T) {
	svc, s, b := setup(t)
	p, _ := s.CreateProject("App", "")
	sess, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code",
		Mode: "wrapper", MCPToken: ptr("tok123")})
	_ = sess
	ch, cancel := b.Subscribe()
	defer cancel()

	cs := connect(t, svc, "tok123")

	out := callText(t, cs, "report_task_completed", map[string]any{
		"summary": "implementei o login", "evidence": "arquivo auth.go criado"})
	if !strings.Contains(strings.ToLower(out), "sugestão") {
		t.Fatalf("report: %s", out)
	}
	select {
	case ev := <-ch:
		if ev.Type != "suggestion.created" {
			t.Fatalf("event: %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("evento não publicado")
	}
	pend, _ := s.ListSuggestions(p.ID, "pending")
	if len(pend) != 1 || pend[0].Type != "add_memory" {
		t.Fatalf("pendentes: %+v", pend)
	}
	if pend[0].SessionID == nil || *pend[0].SessionID != sess.ID {
		t.Fatalf("sem atribuição de sessão: %+v", pend[0])
	}

	callText(t, cs, "report_correction", map[string]any{
		"what_failed": "npm test sem flag", "what_worked": "usar --runInBand"})
	callText(t, cs, "propose_skill", map[string]any{
		"name": "Deploy staging", "draft": "# objetivo..."})
	sk, _ := s.CreateSkill(p.ID, "Existente", "# v1")
	callText(t, cs, "propose_skill_update", map[string]any{
		"skill_id": sk.ID, "diff": "adicionar passo X"})
	callText(t, cs, "append_memory_suggestion", map[string]any{
		"content": "- segredo FOO via op read"})

	pend, _ = s.ListSuggestions(p.ID, "pending")
	if len(pend) != 5 {
		t.Fatalf("esperava 5 sugestões, veio %d", len(pend))
	}
	types := map[string]int{}
	for _, sg := range pend {
		types[sg.Type]++
	}
	if types["add_memory"] != 2 || types["add_correction"] != 1 ||
		types["create_skill"] != 1 || types["update_skill"] != 1 {
		t.Fatalf("tipos: %+v", types)
	}
}

func TestReportToolsExternalNeedProject(t *testing.T) {
	svc, s, _ := setup(t)
	p, _ := s.CreateProject("App", "")
	cs := connect(t, svc, "") // externo, sem token

	out := callText(t, cs, "report_correction", map[string]any{
		"what_failed": "x", "what_worked": "y"})
	if !strings.Contains(out, "project_id") {
		t.Fatalf("devia exigir project_id: %s", out)
	}
	out = callText(t, cs, "report_correction", map[string]any{
		"project_id": p.ID, "what_failed": "x", "what_worked": "y"})
	if strings.Contains(strings.ToLower(out), "erro") {
		t.Fatalf("com project_id devia funcionar: %s", out)
	}
	pend, _ := s.ListSuggestions(p.ID, "pending")
	if len(pend) != 1 {
		t.Fatalf("pendentes: %d", len(pend))
	}
}

// ptr é helper compartilhado pelos testes do pacote (definido aqui, único lugar).
func ptr(s string) *string { return &s }
