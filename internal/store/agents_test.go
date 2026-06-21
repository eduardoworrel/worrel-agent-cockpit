package store

import "testing"

func TestAgentsCRUD(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	a, err := s.CreateAgent(p.ID, "Revisor Go", "Você é um revisor Go rigoroso.", "sess1")
	if err != nil || a.ID == "" || a.Slug == "" {
		t.Fatalf("create: %v %+v", err, a)
	}
	got, err := s.GetAgent(a.ID)
	if err != nil || got.Persona != "Você é um revisor Go rigoroso." {
		t.Fatalf("get: %v %+v", err, got)
	}
	list, _ := s.ListAgents(p.ID)
	if len(list) != 1 {
		t.Fatalf("list=%d", len(list))
	}
	if err := s.DeleteAgent(a.ID); err != nil {
		t.Fatal(err)
	}
	list, _ = s.ListAgents(p.ID)
	if len(list) != 0 {
		t.Fatalf("after delete list=%d", len(list))
	}
}
