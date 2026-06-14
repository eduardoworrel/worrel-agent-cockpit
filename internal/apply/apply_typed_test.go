package apply

import (
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func TestApplySkillLearned(t *testing.T) {
	a, s := setup(t)
	p, _ := s.CreateProject("App", "")
	sg, _ := s.CreateSuggestion(&store.Suggestion{ProjectID: p.ID, Type: "skill.learned",
		Title: "Deploy", Payload: `{"name":"Deploy","content":"# passos"}`})
	if err := a.Accept(sg.ID); err != nil {
		t.Fatal(err)
	}
	skills, _ := s.ListSkills(p.ID)
	if len(skills) != 1 || skills[0].Name != "Deploy" {
		t.Fatalf("skills %+v", skills)
	}
}

func TestApplySkillCorrection(t *testing.T) {
	a, s := setup(t)
	p, _ := s.CreateProject("App", "")
	sk, _ := s.CreateSkill(p.ID, "Deploy", "# v1")
	sg, _ := s.CreateSuggestion(&store.Suggestion{ProjectID: p.ID, Type: "skill.correction",
		SkillID: &sk.ID, Title: "Deploy fix", Payload: `{"name":"Deploy","content":"# v2"}`})
	if err := a.Accept(sg.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetSkill(sk.ID)
	if got.Content != "# v2" {
		t.Fatalf("content %q", got.Content)
	}
}

func TestApplySkillCorrectionRequiresSkillID(t *testing.T) {
	a, s := setup(t)
	p, _ := s.CreateProject("App", "")
	sg, _ := s.CreateSuggestion(&store.Suggestion{ProjectID: p.ID, Type: "skill.correction",
		Title: "sem id", Payload: `{"name":"X","content":"y"}`})
	if err := a.Accept(sg.ID); err == nil {
		t.Fatal("esperava erro sem skill_id")
	}
}

func TestApplySkillVariantCreatesNew(t *testing.T) {
	a, s := setup(t)
	p, _ := s.CreateProject("App", "")
	parent, _ := s.CreateSkill(p.ID, "Deploy", "# base")
	sg, _ := s.CreateSuggestion(&store.Suggestion{ProjectID: p.ID, Type: "skill.variant",
		Title: "Deploy canário", Payload: `{"name":"Deploy canário","content":"# variante","parent_skill_ids":["` + parent.ID + `"]}`})
	if err := a.Accept(sg.ID); err != nil {
		t.Fatal(err)
	}
	skills, _ := s.ListSkills(p.ID)
	if len(skills) != 2 { // base + variante (NÃO sobrescreve a mãe)
		t.Fatalf("skills = %d", len(skills))
	}
}
