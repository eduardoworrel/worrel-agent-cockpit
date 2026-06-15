package store

import "testing"

// TestResetAll: após criar dados, ResetAll esvazia as tabelas e o store volta a
// operar normalmente (schema preservado: dá pra criar projeto de novo).
func TestResetAll(t *testing.T) {
	s, err := Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	p, err := s.CreateProject("App", "desc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveMemory(p.ID, "# Mem", "init"); err != nil {
		t.Fatal(err)
	}

	if err := s.ResetAll(); err != nil {
		t.Fatalf("ResetAll: %v", err)
	}

	list, err := s.ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("esperava 0 projetos após reset, veio %d", len(list))
	}

	// schema preservado: criar de novo funciona
	if _, err := s.CreateProject("App2", ""); err != nil {
		t.Fatalf("CreateProject após reset: %v", err)
	}
}
