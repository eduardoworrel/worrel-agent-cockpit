package store

import "testing"

func TestMemoryVersioning(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	if m, err := s.GetMemory(p.ID); err != nil || m.Content != "" {
		t.Fatalf("memória inicial deve ser vazia: %+v %v", m, err)
	}
	if _, err := s.SaveMemory(p.ID, "# v1", "primeira"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveMemory(p.ID, "# v2", "segunda"); err != nil {
		t.Fatal(err)
	}
	m, _ := s.GetMemory(p.ID)
	if m.Content != "# v2" {
		t.Fatalf("content = %q", m.Content)
	}
	vs, _ := s.ListMemoryVersions(p.ID)
	if len(vs) != 2 {
		t.Fatalf("versões = %d", len(vs))
	}
	if vs[1].Content != "# v1" { // vs[0] é a mais nova, vs[1] a mais antiga
		t.Fatalf("vs[1].Content = %q, want %q", vs[1].Content, "# v1")
	}
	// revert cria nova versão com conteúdo da antiga
	if _, err := s.RevertMemory(p.ID, vs[1].ID); err != nil {
		t.Fatal(err)
	}
	m, _ = s.GetMemory(p.ID)
	if m.Content != "# v1" {
		t.Fatalf("após revert content = %q", m.Content)
	}
}
