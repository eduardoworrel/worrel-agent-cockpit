package store

import "testing"

func TestPendingSweepSessions(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	ended, _ := s.CreateSession(&Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "observed"})
	s.EndSession(ended.ID)
	active, _ := s.CreateSession(&Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	_ = active // ativa não entra na fila
	pend, err := s.PendingSweepSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(pend) != 1 || pend[0].ID != ended.ID {
		t.Fatalf("pend %+v", pend)
	}
	if err := s.MarkSessionAnalyzed(ended.ID); err != nil {
		t.Fatal(err)
	}
	pend, _ = s.PendingSweepSessions()
	if len(pend) != 0 {
		t.Fatalf("após marcar: %d", len(pend))
	}
}

func TestRecentlyUpdatedSkills(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	s.CreateSkill(p.ID, "Recente", "x")
	skills, err := s.RecentlyUpdatedSkills(24)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("skills = %d", len(skills))
	}
}
