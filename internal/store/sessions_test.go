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
