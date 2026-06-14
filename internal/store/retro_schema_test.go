package store

import "testing"

func TestRetroSchema(t *testing.T) {
	s := newTestStore(t)
	var n int
	err := s.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN
		('retro_runs','retro_run_sessions','retro_clusters','secret_suppressions')`).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Fatalf("tabelas retro = %d, want 4", n)
	}
	// coluna origin via migrateAddColumns
	var c int
	if err := s.db.QueryRow(`SELECT count(*) FROM pragma_table_info('suggestions') WHERE name='origin'`).Scan(&c); err != nil {
		t.Fatal(err)
	}
	if c != 1 {
		t.Fatal("coluna suggestions.origin ausente")
	}
}
