package engine_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/engine"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/engine/example"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func TestRunExampleCounterCreatesSuggestion(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := st.CreateProject("App", "")
	sess, _ := st.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	_ = st.AppendTranscriptEvent(sess.ID, "user", "text", "oi", 0, 0)
	_ = st.AppendTranscriptEventRich(sess.ID, "assistant", "tool_use", "Bash ls", `{"name":"Bash"}`, 0, 0)

	r := engine.NewRegistry()
	r.Register(example.Counter{})

	if err := r.Run(context.Background(), st, "example-counter", p.ID, sess.ID); err != nil {
		t.Fatal(err)
	}
	sgs, _ := st.ListSuggestions("", "")
	if len(sgs) != 1 {
		t.Fatalf("esperava 1 sugestão, got %d", len(sgs))
	}
	if sgs[0].Origin != "engine:example-counter" {
		t.Fatalf("origin=%q", sgs[0].Origin)
	}

	if err := r.Run(context.Background(), st, "nope", p.ID, sess.ID); err == nil {
		t.Fatal("motor inexistente deveria erro")
	}
}
