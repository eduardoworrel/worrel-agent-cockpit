package store

import "testing"

func TestAgentGenerations(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	a, _ := s.CreateAgent(p.ID, "Revisor", "persona v1", "sess1")

	// CreateAgent já deve ter semeado a geração 1
	gens, _ := s.ListAgentGenerations(a.ID)
	if len(gens) != 1 || gens[0].Generation != 1 || gens[0].Persona != "persona v1" {
		t.Fatalf("seed gen1: %+v", gens)
	}
	if got, _ := s.GetAgent(a.ID); got.ActiveGeneration != 1 {
		t.Fatalf("active_generation inicial=%d", got.ActiveGeneration)
	}

	// refino → geração 2, persona ativa atualizada
	g, err := s.AddAgentGeneration(a.ID, "persona v2", "ajuste de tom", "sess2")
	if err != nil || g.Generation != 2 {
		t.Fatalf("addgen: %v %+v", err, g)
	}
	got, _ := s.GetAgent(a.ID)
	if got.Persona != "persona v2" || got.ActiveGeneration != 2 {
		t.Fatalf("após refino: persona=%q gen=%d", got.Persona, got.ActiveGeneration)
	}
	if gens, _ := s.ListAgentGenerations(a.ID); len(gens) != 2 {
		t.Fatalf("gens=%d", len(gens))
	}
}
