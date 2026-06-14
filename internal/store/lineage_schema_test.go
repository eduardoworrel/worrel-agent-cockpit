package store

import "testing"

func TestLineageSchema(t *testing.T) {
	s := newTestStore(t)
	tables := []string{"skill_generations", "skill_usage"}
	for _, tb := range tables {
		var n int
		if err := s.db.QueryRow(
			`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, tb).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("tabela %s ausente", tb)
		}
	}
	cols := map[string][]string{
		"skills": {"active_generation", "evolution_policy", "origin"},
	}
	for tb, want := range cols {
		for _, c := range want {
			var n int
			if err := s.db.QueryRow(
				`SELECT count(*) FROM pragma_table_info(?) WHERE name=?`, tb, c).Scan(&n); err != nil {
				t.Fatal(err)
			}
			if n != 1 {
				t.Fatalf("coluna %s.%s ausente", tb, c)
			}
		}
	}
}
