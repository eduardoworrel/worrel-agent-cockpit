package store

import "testing"

func TestSecretsSchema(t *testing.T) {
	s := newTestStore(t)
	var n int
	err := s.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table'
		AND name IN ('secrets','secret_audit')`).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("tabelas de segredo = %d, want 2", n)
	}
}
