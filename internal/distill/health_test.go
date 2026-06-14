package distill

import (
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func TestHealthCheckerDetectsFailures(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sk, _ := s.CreateSkill(p.ID, "Deploy", "# v1")

	// 3 falhas consecutivas
	for i := 0; i < 3; i++ {
		uid, _ := s.RecordSkillUsageStart(sk.ID, nil, 1)
		_ = s.CloseSkillUsage(uid, "error", 1, false, 50)
	}

	hc := NewHealthChecker(s, 2)
	signals, err := hc.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) == 0 {
		t.Fatal("esperava pelo menos 1 sinal de saúde")
	}
	found := false
	for _, sig := range signals {
		if sig.SkillID == sk.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("skill %s não encontrada nos sinais: %+v", sk.ID, signals)
	}
}

func TestHealthCheckerNoPendingIfPolicyClosed(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sk, _ := s.CreateSkill(p.ID, "Deploy", "# v1")

	// política manual → não gera sugestões proativas
	_ = s.SetSkillPolicy(sk.ID, "manual")

	for i := 0; i < 3; i++ {
		uid, _ := s.RecordSkillUsageStart(sk.ID, nil, 1)
		_ = s.CloseSkillUsage(uid, "error", 1, false, 50)
	}

	hc := NewHealthChecker(s, 2)
	signals, err := hc.Scan()
	if err != nil {
		t.Fatal(err)
	}
	// manual policy: sinais ainda são retornados (para exibição), mas sem auto-sugestão
	for _, sig := range signals {
		if sig.SkillID == sk.ID && sig.NeedsAutoCorrect {
			t.Fatalf("policy manual não deve marcar NeedsAutoCorrect")
		}
	}
}

// Critério 4: após 2 falhas consecutivas, uma sugestão skill.correction
// proativa aparece com diagnóstico em evidence, sem ação do usuário.
func TestHealthCreatesProactiveCorrection(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sk, _ := s.CreateSkill(p.ID, "Deploy", "# v1")
	for i := 0; i < 2; i++ {
		uid, _ := s.RecordSkillUsageStart(sk.ID, nil, 1)
		_ = s.CloseSkillUsage(uid, "error", 1, false, 50)
	}
	hc := NewHealthChecker(s, 2)
	n, err := hc.CreateProactiveCorrections(nil, nil, 3)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("correções proativas criadas = %d, want 1", n)
	}
	pend, _ := s.ListSuggestions(p.ID, "pending")
	if len(pend) != 1 || pend[0].Type != "skill.correction" || pend[0].SkillID == nil {
		t.Fatalf("sugestão proativa inesperada: %+v", pend)
	}
	if pend[0].Evidence == "" {
		t.Fatal("sugestão proativa deve conter diagnóstico em evidence")
	}
	// Idempotente: segunda varredura não duplica.
	n2, _ := hc.CreateProactiveCorrections(nil, nil, 3)
	if n2 != 0 {
		t.Fatalf("duplicou correção proativa: %d", n2)
	}
}

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
