package mcpserver

import (
	"testing"
)

// TestLoadSkillRecordsUsageStart prova que load_skill registra início de uso
// (TotalUses=0 enquanto uso está aberto). O fechamento via report_task_completed
// foi removido na demolição sp1.
func TestLoadSkillRecordsUsageStart(t *testing.T) {
	svc, s, _ := setup(t)
	p, _ := s.CreateProject("App", "")
	sk, _ := s.CreateSkill(p.ID, "Deploy", "# Objetivo\nfazer deploy")

	cs := connect(t, svc, "")

	callText(t, cs, "load_skill", map[string]any{"skill_id": sk.ID})
	stats, _ := s.SkillStats(sk.ID)
	if stats.TotalUses != 0 {
		t.Fatalf("uso ainda aberto não deve contar como total resolvido: %+v", stats)
	}
}
