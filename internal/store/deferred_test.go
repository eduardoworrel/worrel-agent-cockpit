package store

import "testing"

func TestDeferredSessionsQueue(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	a, _ := s.CreateSession(&Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	b, _ := s.CreateSession(&Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})

	// Nada adiado ainda.
	if list, _ := s.ListDeferredSessions(); len(list) != 0 {
		t.Fatalf("esperava fila vazia, veio %+v", list)
	}

	// Adia a, depois b → b é a mais recente (primeira na ordem desc).
	if err := s.SetSessionDeferred(a.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSessionDeferred(b.ID); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListDeferredSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("esperava 2 adiadas, veio %d", len(list))
	}
	if list[0].SessionID != b.ID || list[1].SessionID != a.ID {
		t.Fatalf("ordem errada (esperava b,a): %+v", list)
	}
	if list[0].Label == "" || list[0].ProjectID != p.ID {
		t.Fatalf("rótulo/projeto faltando: %+v", list[0])
	}

	// Responder limpa a marca: a sai da fila.
	if err := s.ClearSessionDeferred(a.ID); err != nil {
		t.Fatal(err)
	}
	list, _ = s.ListDeferredSessions()
	if len(list) != 1 || list[0].SessionID != b.ID {
		t.Fatalf("esperava só b após limpar a: %+v", list)
	}
}
