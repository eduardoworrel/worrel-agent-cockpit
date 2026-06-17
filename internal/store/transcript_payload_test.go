package store

import "testing"

func TestTranscriptPayloadRoundTrip(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sess, _ := s.CreateSession(&Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})

	const pl = `{"name":"Bash","input":{"command":"ls"}}`
	if err := s.AppendTranscriptEventRich(sess.ID, "assistant", "tool_use", "Bash ls", pl, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendTranscriptEvent(sess.ID, "user", "text", "oi", 0, 0); err != nil {
		t.Fatal(err)
	}

	evs, err := s.ListTranscriptEvents(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 {
		t.Fatalf("len=%d", len(evs))
	}
	if evs[0].Payload != pl {
		t.Fatalf("rich payload=%q", evs[0].Payload)
	}
	if evs[1].Payload != "" {
		t.Fatalf("plain payload deveria ser vazio, got %q", evs[1].Payload)
	}
}
