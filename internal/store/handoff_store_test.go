package store

import "testing"

func TestArchiveAndChain(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	old, _ := s.CreateSession(&Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})

	if err := s.SetSessionSummary(old.ID, "## Estado atual\n..."); err != nil {
		t.Fatal(err)
	}
	if err := s.ArchiveSession(old.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetSession(old.ID)
	if got.Status != "archived" || got.Summary == "" {
		t.Fatalf("arquivamento: %+v", got)
	}

	cont := old.ID
	newSess, _ := s.CreateSession(&Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper", Continues: &cont})

	// A nova aponta para a antiga.
	if newSess.Continues == nil || *newSess.Continues != old.ID {
		t.Fatalf("continues errado: %+v", newSess.Continues)
	}
	// A antiga é "continuada por" a nova.
	by, err := s.ContinuedBy(old.ID)
	if err != nil || by == nil || *by != newSess.ID {
		t.Fatalf("ContinuedBy = %v %v", by, err)
	}
}
