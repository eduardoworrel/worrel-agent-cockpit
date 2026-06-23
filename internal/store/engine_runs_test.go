package store

import "testing"

func TestEngineRunsWatermark(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	// uma sessão encerrada, uma ativa
	ended, _ := s.CreateSession(&Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper", Status: "ended"})
	_, _ = s.CreateSession(&Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper", Status: "active"})

	un, err := s.UnrunEndedSessions("memory", "")
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
	if un, _ := s.UnrunEndedSessions("memory", ""); len(un) != 0 {
		t.Fatalf("após marcar: %d", len(un))
	}
	// mas outro motor ainda a vê
	if un, _ := s.UnrunEndedSessions("skill", ""); len(un) != 1 {
		t.Fatalf("outro motor: %d", len(un))
	}
}

func TestUnrunEndedByProjectAndCount(t *testing.T) {
	s := newTestStore(t)
	pa, _ := s.CreateProject("A", "")
	pb, _ := s.CreateProject("B", "")
	ea, _ := s.CreateSession(&Session{ProjectID: pa.ID, Adapter: "claude-code", Mode: "wrapper", Status: "ended"})
	_, _ = s.CreateSession(&Session{ProjectID: pb.ID, Adapter: "claude-code", Mode: "wrapper", Status: "ended"})
	_, _ = s.CreateSession(&Session{ProjectID: pa.ID, Adapter: "claude-code", Mode: "wrapper", Status: "active"})

	// sem filtro: 2 encerradas não-analisadas
	if n, err := s.CountUnrunEndedSessions("memory", ""); err != nil || n != 2 {
		t.Fatalf("count global = %d, err=%v (quer 2)", n, err)
	}
	// filtro por projeto A: só a sessão de A
	un, err := s.UnrunEndedSessions("memory", pa.ID)
	if err != nil || len(un) != 1 || un[0].ID != ea.ID {
		t.Fatalf("unrun A = %+v, err=%v", un, err)
	}
	if n, _ := s.CountUnrunEndedSessions("memory", pa.ID); n != 1 {
		t.Fatalf("count A = %d (quer 1)", n)
	}
	// após marcar a de A, count A zera; global cai p/ 1
	_ = s.MarkEngineRun("memory", ea.ID)
	if n, _ := s.CountUnrunEndedSessions("memory", pa.ID); n != 0 {
		t.Fatalf("count A pós-marca = %d (quer 0)", n)
	}
	if n, _ := s.CountUnrunEndedSessions("memory", ""); n != 1 {
		t.Fatalf("count global pós-marca = %d (quer 1)", n)
	}
}
