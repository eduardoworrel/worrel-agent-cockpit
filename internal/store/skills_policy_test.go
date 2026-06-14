package store

import "testing"

func TestSkillPolicyAndOrigin(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sk, _ := s.CreateSkill(p.ID, "Deploy", "# v1")
	if sk.EvolutionPolicy != "manual" || sk.ActiveGeneration != 1 {
		t.Fatalf("defaults: %+v", sk)
	}
	if err := s.SetSkillPolicy(sk.ID, "auto_correction"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetSkill(sk.ID)
	if got.EvolutionPolicy != "auto_correction" {
		t.Fatalf("policy = %q", got.EvolutionPolicy)
	}
	if err := s.SetProjectSkillsPolicy(p.ID, "manual"); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetSkill(sk.ID)
	if got.EvolutionPolicy != "manual" {
		t.Fatalf("bulk policy = %q", got.EvolutionPolicy)
	}
}
