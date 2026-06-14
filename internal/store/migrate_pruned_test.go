package store

import "testing"

func TestTranscriptPrunedColumnIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/t.db"

	s1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !hasColumn(t, s1, "sessions", "transcript_pruned") {
		t.Fatal("coluna transcript_pruned não criada na 1ª abertura")
	}
	s1.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reabertura falhou (migração não-idempotente): %v", err)
	}
	defer s2.Close()
	if !hasColumn(t, s2, "sessions", "transcript_pruned") {
		t.Fatal("coluna sumiu na reabertura")
	}
}

func hasColumn(t *testing.T, s *Store, table, col string) bool {
	t.Helper()
	rows, err := s.db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == col {
			return true
		}
	}
	return false
}
