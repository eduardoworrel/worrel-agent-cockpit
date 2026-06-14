package store

import "testing"

func TestPruneTranscriptPreservesMetadataAndSuggestions(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sess, _ := s.CreateSession(&Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper", Title: "Trabalho importante"})
	_, _ = s.SaveMemory(p.ID, "# mem", "init")
	_ = s.AppendTranscriptEvent(sess.ID, "user", "text", "olá", 0, 0)
	_ = s.AppendTranscriptEvent(sess.ID, "assistant", "text", "oi", 0, 0)

	sid := sess.ID
	sg, _ := s.CreateSuggestion(&Suggestion{ProjectID: p.ID, SessionID: &sid,
		Type: "create_skill", Title: "Deploy", Evidence: "trecho relevante copiado"})

	if err := s.PruneSessionTranscript(sess.ID); err != nil {
		t.Fatal(err)
	}

	evs, _ := s.ListTranscriptEvents(sess.ID)
	if len(evs) != 0 {
		t.Fatalf("transcript não foi podado: %d eventos", len(evs))
	}
	got, _ := s.GetSession(sess.ID)
	if !got.TranscriptPruned {
		t.Fatal("flag transcript_pruned não marcada")
	}
	if got.Title != "Trabalho importante" || got.ProjectID != p.ID {
		t.Fatalf("metadados perdidos: %+v", got)
	}
	gotSg, err := s.GetSuggestion(sg.ID)
	if err != nil || gotSg.Evidence != "trecho relevante copiado" {
		t.Fatalf("evidência da sugestão perdida: %+v %v", gotSg, err)
	}
}

func TestExpiredSessionIDs(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	old, _ := s.CreateSession(&Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	recent, _ := s.CreateSession(&Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})

	if err := s.SetSessionTimesForTest(old.ID, 40*24*60*60*1000); err != nil {
		t.Fatal(err)
	}
	_ = s.AppendTranscriptEvent(old.ID, "user", "text", "antigo", 0, 0)
	_ = s.AppendTranscriptEvent(recent.ID, "user", "text", "novo", 0, 0)

	cutoff := now() - 30*24*60*60*1000
	ids, err := s.ExpiredSessionIDs(cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != old.ID {
		t.Fatalf("expirados = %v, want [%s]", ids, old.ID)
	}
}
