package store

import (
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenMigrates(t *testing.T) {
	s := newTestStore(t)
	var n int
	err := s.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN
		('projects','project_dirs','memory_versions','skills','sessions','transcript_events','suggestions','settings')`).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 8 {
		t.Fatalf("tabelas = %d, want 8", n)
	}
}
