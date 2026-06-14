package mcpserver

import (
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// TestLoadSkillRecordsUsageAndReportCloses prova o caminho de produção da
// coleta de métricas (spec §4.1): load_skill registra início de uso na sessão,
// e report_task_completed com skill_id fecha o desfecho — sem o qual o gatilho
// de saúde (critério 4) nunca dispara no app real.
func TestLoadSkillRecordsUsageAndReportCloses(t *testing.T) {
	svc, s, _ := setup(t)
	p, _ := s.CreateProject("App", "")
	sk, _ := s.CreateSkill(p.ID, "Deploy", "# Objetivo\nfazer deploy")
	tok := "tok-usage-1"
	sess, err := s.CreateSession(&store.Session{
		ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper", MCPToken: &tok,
	})
	if err != nil {
		t.Fatal(err)
	}

	cs := connect(t, svc, tok)

	// load_skill → registra início de uso na sessão.
	callText(t, cs, "load_skill", map[string]any{"skill_id": sk.ID})
	stats, _ := s.SkillStats(sk.ID)
	if stats.TotalUses != 0 {
		t.Fatalf("uso ainda aberto não deve contar como total resolvido: %+v", stats)
	}

	// report_task_completed com skill_id + outcome=error → fecha desfecho.
	callText(t, cs, "report_task_completed", map[string]any{
		"project_id": p.ID, "summary": "tentou deploy", "skill_id": sk.ID, "outcome": "error",
	})
	stats, _ = s.SkillStats(sk.ID)
	if stats.TotalUses != 1 || stats.ErrorCount != 1 {
		t.Fatalf("desfecho não fechado corretamente: %+v", stats)
	}
	n, _ := s.ConsecutiveFailures(sk.ID)
	if n != 1 {
		t.Fatalf("falha consecutiva = %d, want 1", n)
	}
	_ = sess
}
