package wrapper

import (
	"path/filepath"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func TestIngestTranscriptIncremental(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	m := New(st, bus.New())
	p, _ := st.CreateProject("App", "")
	sess, _ := st.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})

	evs := []adapter.TranscriptEvent{
		{Role: "user", Kind: "text", Content: "oi"},
		{Role: "assistant", Kind: "tool_use", Content: "Bash ls", Payload: `{"name":"Bash"}`},
	}
	if n, err := m.ingestTranscript(sess.ID, evs); err != nil || n != 2 {
		t.Fatalf("primeira: n=%d err=%v", n, err)
	}

	// nova leitura com 1 evento a mais: só o novo é apendado
	evs = append(evs, adapter.TranscriptEvent{Role: "user", Kind: "text", Content: "tchau"})
	if n, err := m.ingestTranscript(sess.ID, evs); err != nil || n != 1 {
		t.Fatalf("incremental: n=%d err=%v", n, err)
	}

	// releitura idêntica: nada novo (idempotente)
	if n, _ := m.ingestTranscript(sess.ID, evs); n != 0 {
		t.Fatalf("idempotente: esperado 0, got %d", n)
	}

	stored, _ := st.ListTranscriptEvents(sess.ID)
	if len(stored) != 3 {
		t.Fatalf("stored len=%d", len(stored))
	}
	if stored[1].Payload != `{"name":"Bash"}` {
		t.Fatalf("payload preservado? got %q", stored[1].Payload)
	}
}
