package httpapi

import (
	"encoding/json"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func TestSessionsExposeTranscriptPruned(t *testing.T) {
	ts, s := newTestServer(t)
	p, _ := s.CreateProject("App", "")
	sess, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	_ = s.AppendTranscriptEvent(sess.ID, "user", "text", "x", 0, 0)
	_ = s.PruneSessionTranscript(sess.ID)

	var list []map[string]any
	code := getJSON(t, ts, "/api/sessions", &list)
	if code != 200 || len(list) != 1 {
		t.Fatalf("code=%d len=%d", code, len(list))
	}
	b, _ := json.Marshal(list[0])
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if v, ok := m["transcript_pruned"]; !ok || v != true {
		t.Fatalf("transcript_pruned ausente/falso no JSON: %v", m)
	}
}
