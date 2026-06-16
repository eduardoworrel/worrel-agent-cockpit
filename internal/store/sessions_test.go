package store

import "testing"

func TestSessionsAndSettings(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sess, err := s.CreateSession(&Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AppendTranscriptEvent(sess.ID, "user", "text", "olá", 0, 0); err != nil {
		t.Fatal(err)
	}
	evs, _ := s.ListTranscriptEvents(sess.ID)
	if len(evs) != 1 || evs[0].Content != "olá" {
		t.Fatalf("evs %+v", evs)
	}
	if err := s.EndSession(sess.ID); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListSessions(p.ID)
	if len(list) != 1 || list[0].Status != "ended" {
		t.Fatalf("list %+v", list)
	}
	if err := s.SetSetting("retention_days", "30"); err != nil {
		t.Fatal(err)
	}
	if v := s.GetSetting("retention_days", "x"); v != "30" {
		t.Fatalf("v = %q", v)
	}
	if v := s.GetSetting("nao-existe", "padrão"); v != "padrão" {
		t.Fatalf("v = %q", v)
	}
}

func TestEndOrphanedWrapperSessions(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")

	// duas wrapper active (órfãs após restart) + uma observed active + uma já ended
	w1, _ := s.CreateSession(&Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	w2, _ := s.CreateSession(&Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	obs, _ := s.CreateSession(&Session{ProjectID: p.ID, Adapter: "mcp", Mode: "observed"})
	done, _ := s.CreateSession(&Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	if err := s.EndSession(done.ID); err != nil {
		t.Fatal(err)
	}

	n, err := s.EndOrphanedWrapperSessions()
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("reconciliadas = %d, quero 2", n)
	}

	// as wrapper viraram ended; a observed segue active
	for _, id := range []string{w1.ID, w2.ID} {
		got, _ := s.GetSession(id)
		if got.Status != "ended" || got.EndedAt == nil {
			t.Fatalf("wrapper %s status=%s ended_at=%v", id, got.Status, got.EndedAt)
		}
	}
	if got, _ := s.GetSession(obs.ID); got.Status != "active" {
		t.Fatalf("observed não deveria ser encerrada, status=%s", got.Status)
	}

	// faixa de ativas deve sair vazia
	active, _ := s.ListActiveWrapperSessions()
	if len(active) != 0 {
		t.Fatalf("active wrapper = %d, quero 0", len(active))
	}

	// idempotente: segunda passada não reconcilia nada
	if n2, _ := s.EndOrphanedWrapperSessions(); n2 != 0 {
		t.Fatalf("segunda passada reconciliou %d, quero 0", n2)
	}
}

func TestSessionByMCPTokenRoundTrip(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	token := "test-mcp-token-123"
	sess, err := s.CreateSession(&Session{
		ProjectID: p.ID, Adapter: "mcp", Mode: "observed", MCPToken: &token,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.SessionByMCPToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != sess.ID {
		t.Fatalf("ID mismatch: got %s want %s", got.ID, sess.ID)
	}
	if got.MCPToken == nil || *got.MCPToken != token {
		t.Fatalf("MCPToken mismatch: got %v want %s", got.MCPToken, token)
	}
}
