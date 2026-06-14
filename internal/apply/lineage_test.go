package apply

import (
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func TestApplySkillCorrectionCreatesGeneration(t *testing.T) {
	a, s := setup(t)
	p, _ := s.CreateProject("App", "")
	sk, _ := s.CreateSkill(p.ID, "Deploy", "# v1")

	// Suggestion tipo skill.correction deve criar geração tipada
	sg, err := s.CreateSuggestion(&store.Suggestion{
		ProjectID: p.ID, Type: "skill.correction",
		SkillID: &sk.ID, Title: "Corrige edge case",
		Payload:  `{"name":"Deploy","content":"# v2","diff":"+v2","change_summary":"corrige edge case","evidence":"trecho"}`,
		Evidence: "trecho",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Accept(sg.ID); err != nil {
		t.Fatal(err)
	}
	gens, err := s.ListGenerations(sk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(gens) != 2 {
		t.Fatalf("esperava 2 gerações, got %d", len(gens))
	}
	if gens[1].EvolutionType != "correction" {
		t.Fatalf("evolution_type = %q", gens[1].EvolutionType)
	}
	got, _ := s.GetSkill(sk.ID)
	if got.Content != "# v2" || got.ActiveGeneration != 2 {
		t.Fatalf("skill desatualizada: %+v", got)
	}
}

func TestApplySkillLearnedCreatesGeneration(t *testing.T) {
	a, s := setup(t)
	p, _ := s.CreateProject("App", "")
	sk, _ := s.CreateSkill(p.ID, "Deploy", "# v1")

	sg, err := s.CreateSuggestion(&store.Suggestion{
		ProjectID: p.ID, Type: "skill.learned",
		SkillID: &sk.ID, Title: "Aprende novo passo",
		Payload: `{"name":"Deploy","content":"# v3","diff":"+v3","change_summary":"novo passo","evidence":"observado"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Accept(sg.ID); err != nil {
		t.Fatal(err)
	}
	gens, _ := s.ListGenerations(sk.ID)
	if len(gens) != 2 {
		t.Fatalf("esperava 2 gerações, got %d", len(gens))
	}
	if gens[1].EvolutionType != "learned" {
		t.Fatalf("evolution_type = %q", gens[1].EvolutionType)
	}
}

func TestApplySkillVariantPersistsLineage(t *testing.T) {
	a, s := setup(t)
	p, _ := s.CreateProject("App", "")
	mother, _ := s.CreateSkill(p.ID, "Deploy", "# base")
	sg, err := s.CreateSuggestion(&store.Suggestion{
		ProjectID: p.ID, Type: "skill.variant",
		Title:    "Deploy Canary",
		Payload:  `{"name":"Deploy Canary","content":"# canary","parent_skill_ids":["` + mother.ID + `"]}`,
		Evidence: "trecho de variação",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Accept(sg.ID); err != nil {
		t.Fatal(err)
	}
	skills, _ := s.ListSkills(p.ID)
	if len(skills) != 2 {
		t.Fatalf("mãe e variante devem coexistir: %d", len(skills))
	}
	var variant *store.Skill
	for _, x := range skills {
		if x.Name == "Deploy Canary" {
			variant = x
		}
	}
	if variant == nil {
		t.Fatal("variante não criada")
	}
	gens, _ := s.ListGenerations(variant.ID)
	if len(gens) != 1 {
		t.Fatalf("variante deve ter 1 geração, got %d", len(gens))
	}
	// Critério 3: gen-1 da variante é tipo 'variant' com a mãe na linhagem.
	if gens[0].EvolutionType != "variant" {
		t.Fatalf("evolution_type = %q, want variant", gens[0].EvolutionType)
	}
	if len(gens[0].ParentSkillIDs) != 1 || gens[0].ParentSkillIDs[0] != mother.ID {
		t.Fatalf("parent_skill_ids = %+v, want [%s]", gens[0].ParentSkillIDs, mother.ID)
	}
	// A mãe continua ativa e íntegra.
	gotMother, _ := s.GetSkill(mother.ID)
	if gotMother.Content != "# base" {
		t.Fatalf("mãe alterada: %q", gotMother.Content)
	}
}

func TestApplySkillVariantFusionTwoMothers(t *testing.T) {
	a, s := setup(t)
	p, _ := s.CreateProject("App", "")
	m1, _ := s.CreateSkill(p.ID, "Deploy", "# a")
	m2, _ := s.CreateSkill(p.ID, "Rollback", "# b")
	sg, _ := s.CreateSuggestion(&store.Suggestion{
		ProjectID: p.ID, Type: "skill.variant",
		Title:   "Deploy seguro",
		Payload: `{"name":"Deploy seguro","content":"# fusao","parent_skill_ids":["` + m1.ID + `","` + m2.ID + `"]}`,
	})
	if err := a.Accept(sg.ID); err != nil {
		t.Fatal(err)
	}
	skills, _ := s.ListSkills(p.ID)
	var variant *store.Skill
	for _, x := range skills {
		if x.Name == "Deploy seguro" {
			variant = x
		}
	}
	gens, _ := s.ListGenerations(variant.ID)
	if gens[0].EvolutionType != "variant" || len(gens[0].ParentSkillIDs) != 2 {
		t.Fatalf("fusão N-mães: %+v", gens[0])
	}
}

func TestApplyAutoAppliedStatus(t *testing.T) {
	a, s := setup(t)
	p, _ := s.CreateProject("App", "")
	sk, _ := s.CreateSkill(p.ID, "Deploy", "# v1")

	sg, err := s.CreateSuggestion(&store.Suggestion{
		ProjectID: p.ID, Type: "skill.correction",
		SkillID: &sk.ID, Title: "Auto corrige",
		Payload: `{"name":"Deploy","content":"# v-auto","authorship":"engine_auto"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.AutoApply(sg.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetSuggestion(sg.ID)
	if got.Status != "auto_applied" {
		t.Fatalf("status = %q", got.Status)
	}
}
