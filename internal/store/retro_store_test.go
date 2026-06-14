package store

import "testing"

func TestRetroRunCursor(t *testing.T) {
	s := newTestStore(t)
	run, err := s.CreateRetroRun(&RetroRun{Depth: "completa", Scope: `{"window_days":60}`, BudgetPerHour: 10})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := s.CreateProject("App", "")
	se, _ := s.CreateSession(&Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "observed"})
	_ = s.EndSession(se.ID)
	if err := s.AddRunSession(run.ID, se.ID, p.ID); err != nil {
		t.Fatal(err)
	}
	pend, _ := s.PendingRunSessions(run.ID)
	if len(pend) != 1 {
		t.Fatalf("pendentes = %d, want 1", len(pend))
	}
	if err := s.MarkRunSessionDone(run.ID, se.ID); err != nil {
		t.Fatal(err)
	}
	pend, _ = s.PendingRunSessions(run.ID)
	if len(pend) != 0 {
		t.Fatalf("após done = %d, want 0", len(pend))
	}
	if err := s.IncrRunLLMCalls(run.ID, 3); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetRetroRun(run.ID)
	if got.LLMCalls != 3 {
		t.Fatalf("llm_calls = %d", got.LLMCalls)
	}
	if err := s.SetRetroRunStatus(run.ID, "paused"); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetRetroRun(run.ID)
	if got.Status != "paused" {
		t.Fatalf("status = %q", got.Status)
	}
}
