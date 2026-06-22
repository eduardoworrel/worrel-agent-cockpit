package store

import "testing"

func TestEngineRunsWatermark(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	// uma sessão encerrada, uma ativa
	ended, _ := s.CreateSession(&Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper", Status: "ended"})
	_, _ = s.CreateSession(&Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper", Status: "active"})

	un, err := s.UnrunEndedSessions("memory")
	if err != nil {
		t.Fatal(err)
	}
	if len(un) != 1 || un[0].ID != ended.ID {
		t.Fatalf("unrun deveria ser só a encerrada: %+v", un)
	}

	if err := s.MarkEngineRun("memory", ended.ID); err != nil {
		t.Fatal(err)
	}
	// agora não há mais sessões não-processadas por "memory"
	if un, _ := s.UnrunEndedSessions("memory"); len(un) != 0 {
		t.Fatalf("após marcar: %d", len(un))
	}
	// mas outro motor ainda a vê
	if un, _ := s.UnrunEndedSessions("skill"); len(un) != 1 {
		t.Fatalf("outro motor: %d", len(un))
	}
}
