package apply

import (
	"path/filepath"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/mirror"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func newApplier(t *testing.T) (*Applier, *store.Store) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	mir := mirror.New(t.TempDir())
	return New(st, mir, bus.New()), st
}

func TestAcceptAddMemoryEntryCoexist(t *testing.T) {
	a, st := newApplier(t)
	p, _ := st.CreateProject("App", "")
	sg, _ := st.CreateSuggestion(&store.Suggestion{ProjectID: p.ID, Type: "add_memory_entry",
		Title: "build", Payload: `{"content":"use go build","category":"convencao"}`})
	if err := a.Accept(sg.ID); err != nil {
		t.Fatal(err)
	}
	entries, _ := st.ListMemoryEntries(p.ID, false)
	if len(entries) != 1 || entries[0].Content != "use go build" {
		t.Fatalf("entries=%+v", entries)
	}
}

func TestAcceptSuperseding(t *testing.T) {
	a, st := newApplier(t)
	p, _ := st.CreateProject("App", "")
	old, _ := st.CreateMemoryEntry(&store.MemoryEntry{ProjectID: p.ID, Content: "use make", Category: "convencao"})
	sg, _ := st.CreateSuggestion(&store.Suggestion{ProjectID: p.ID, Type: "add_memory_entry",
		Title: "build", Payload: `{"content":"use go build","category":"convencao"}`})
	if err := a.AcceptSuperseding(sg.ID, old.ID); err != nil {
		t.Fatal(err)
	}
	active, _ := st.ListMemoryEntries(p.ID, false)
	if len(active) != 1 || active[0].Content != "use go build" {
		t.Fatalf("active=%+v", active)
	}
}
