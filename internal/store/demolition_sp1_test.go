package store

import "testing"

func TestDemolitionSP1WipesDerivedKeepsSessions(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sess, _ := s.CreateSession(&Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	if err := s.AppendTranscriptEvent(sess.ID, "user", "text", "oi", 0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSuggestion(&Suggestion{ProjectID: p.ID, Type: "add_memory", Title: "x"}); err != nil {
		t.Fatal(err)
	}
	_ = s.SetSetting("headless_adapter", "opencode")

	if err := s.migrateDemolitionSP1(); err != nil {
		t.Fatal(err)
	}

	// sessões e transcript preservados
	if got, _ := s.GetSession(sess.ID); got == nil {
		t.Fatal("sessão sumiu")
	}
	if evs, _ := s.ListTranscriptEvents(sess.ID); len(evs) != 1 {
		t.Fatalf("transcript apagado: %d", len(evs))
	}
	// derivados zerados
	if sgs, _ := s.ListSuggestions("", ""); len(sgs) != 0 {
		t.Fatalf("sugestões não zeradas: %d", len(sgs))
	}
	// setting de distill removido
	if v := s.GetSetting("headless_adapter", "GONE"); v != "GONE" {
		t.Fatalf("setting de distill sobreviveu: %q", v)
	}
	// idempotente: rodar de novo não explode
	if err := s.migrateDemolitionSP1(); err != nil {
		t.Fatalf("segunda passada: %v", err)
	}
}
