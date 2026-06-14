package apply

import (
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func TestAutoCorrectionAppliesWithoutQueue(t *testing.T) {
	a, s := setup(t)
	p, _ := s.CreateProject("App", "")
	sk, _ := s.CreateSkill(p.ID, "Deploy", "# v1")
	_ = s.SetSkillPolicy(sk.ID, "auto_correction")
	sg, _ := s.CreateSuggestion(&store.Suggestion{ProjectID: p.ID, Type: "skill.correction",
		SkillID: &sk.ID, Title: "fix", Payload: `{"name":"Deploy","content":"# v2"}`, Evidence: "x"})
	applied, err := a.MaybeAutoApply(sg.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !applied {
		t.Fatal("deveria auto-aplicar correção com política auto_correction")
	}
	got, _ := s.GetSuggestion(sg.ID)
	if got.Status != "auto_applied" {
		t.Fatalf("status = %q, want auto_applied", got.Status)
	}
	gens, _ := s.ListGenerations(sk.ID)
	if gens[len(gens)-1].Authorship != "engine_auto" {
		t.Fatalf("autoria = %q, want engine_auto", gens[len(gens)-1].Authorship)
	}
}

// Critério 6: um Aprendizado (criação inicial, sem skill_id) NUNCA é automático.
func TestAutoLearnedInitialStaysManual(t *testing.T) {
	a, s := setup(t)
	p, _ := s.CreateProject("App", "")
	sg, _ := s.CreateSuggestion(&store.Suggestion{ProjectID: p.ID, Type: "skill.learned",
		Title: "Nova", Payload: `{"name":"Nova","content":"# x"}`, Evidence: "x"})
	applied, _ := a.MaybeAutoApply(sg.ID, 3)
	if applied {
		t.Fatal("aprendizado inicial nunca é automático")
	}
	got, _ := s.GetSuggestion(sg.ID)
	if got.Status != "pending" {
		t.Fatalf("status = %q, want pending", got.Status)
	}
}

// Critério 6: correção com política manual permanece na fila (não auto-aplica).
func TestAutoCorrectionManualStaysPending(t *testing.T) {
	a, s := setup(t)
	p, _ := s.CreateProject("App", "")
	sk, _ := s.CreateSkill(p.ID, "Deploy", "# v1") // policy default = manual
	sg, _ := s.CreateSuggestion(&store.Suggestion{ProjectID: p.ID, Type: "skill.correction",
		SkillID: &sk.ID, Title: "fix", Payload: `{"name":"Deploy","content":"# v2"}`})
	applied, _ := a.MaybeAutoApply(sg.ID, 3)
	if applied {
		t.Fatal("política manual não deve auto-aplicar")
	}
	got, _ := s.GetSuggestion(sg.ID)
	if got.Status != "pending" {
		t.Fatalf("status = %q, want pending", got.Status)
	}
}

func TestMaybeAutoApplyRespectsCap(t *testing.T) {
	a, s := setup(t)
	p, _ := s.CreateProject("App", "")
	sk, _ := s.CreateSkill(p.ID, "Deploy", "# v1")

	_ = s.SetSkillPolicy(sk.ID, "auto_correction")

	// Create suggestions
	sgs := make([]*store.Suggestion, 0)
	for i := 0; i < 3; i++ {
		sg, _ := s.CreateSuggestion(&store.Suggestion{
			ProjectID: p.ID, Type: "skill.correction",
			SkillID: &sk.ID, Title: "fix",
			Payload: `{"name":"Deploy","content":"# auto"}`,
		})
		sgs = append(sgs, sg)
	}

	applied, err := a.MaybeAutoApply(sgs[0].ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !applied {
		t.Fatal("esperava aplicação automática")
	}

	// Manually seed a generation to simulate having already auto-applied today
	for i := 0; i < 2; i++ {
		_, _ = s.AddGeneration(sk.ID, store.GenerationInput{
			EvolutionType: "correction", Snapshot: "# x",
			Authorship: "engine_auto",
		})
	}

	applied2, err := a.MaybeAutoApply(sgs[1].ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if applied2 {
		t.Fatal("cap diário deve impedir mais auto-aplicações")
	}
}

