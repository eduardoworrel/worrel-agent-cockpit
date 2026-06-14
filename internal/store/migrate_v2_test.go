package store

import "testing"

func TestMigrateV2SeedsGenerations(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	_, err := s.db.Exec(`INSERT INTO skills (id, project_id, slug, name, content, created_at, updated_at)
		VALUES ('legacy', ?, 'legada', 'Legada', '# antiga', ?, ?)`, p.ID, now(), now())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MigrateSkillsToLineage(); err != nil {
		t.Fatal(err)
	}
	gens, _ := s.ListGenerations("legacy")
	if len(gens) != 1 || gens[0].EvolutionType != "learned" || gens[0].Snapshot != "# antiga" {
		t.Fatalf("migração: %+v", gens)
	}
	if err := s.MigrateSkillsToLineage(); err != nil {
		t.Fatal(err)
	}
	gens, _ = s.ListGenerations("legacy")
	if len(gens) != 1 {
		t.Fatalf("não-idempotente: %d", len(gens))
	}
}
