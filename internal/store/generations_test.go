package store

import "testing"

func TestGenerationsLifecycle(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sk, _ := s.CreateSkill(p.ID, "Deploy", "# v1")

	gens, _ := s.ListGenerations(sk.ID)
	if len(gens) != 1 || gens[0].Generation != 1 || gens[0].EvolutionType != "learned" {
		t.Fatalf("geração inicial: %+v", gens)
	}

	g2, err := s.AddGeneration(sk.ID, GenerationInput{
		EvolutionType: "correction", Snapshot: "# v2", Diff: "+v2",
		ChangeSummary: "corrige edge case", Evidence: "trecho", Authorship: "engine_approved",
	})
	if err != nil {
		t.Fatal(err)
	}
	if g2.Generation != 2 {
		t.Fatalf("gen = %d", g2.Generation)
	}
	got, _ := s.GetSkill(sk.ID)
	if got.Content != "# v2" || got.ActiveGeneration != 2 {
		t.Fatalf("cache desatualizado: %+v", got)
	}

	if err := s.ActivateGeneration(sk.ID, 1); err != nil {
		t.Fatal(err)
	}
	gens, _ = s.ListGenerations(sk.ID)
	if len(gens) != 2 {
		t.Fatalf("reverter não pode criar geração: %d", len(gens))
	}
	got, _ = s.GetSkill(sk.ID)
	if got.Content != "# v1" || got.ActiveGeneration != 1 {
		t.Fatalf("revert não restaurou cache: %+v", got)
	}
}
