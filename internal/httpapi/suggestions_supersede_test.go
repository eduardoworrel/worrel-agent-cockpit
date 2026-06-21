package httpapi

import (
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/apply"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/mirror"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func TestAcceptSuggestionWithSupersede(t *testing.T) {
	st, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	p, _ := st.CreateProject("App", "")
	old, _ := st.CreateMemoryEntry(&store.MemoryEntry{ProjectID: p.ID, Content: "use make", Category: "convencao"})
	sg, _ := st.CreateSuggestion(&store.Suggestion{ProjectID: p.ID, Type: "add_memory_entry",
		Title: "build", Payload: `{"content":"use go build","category":"convencao"}`})
	srv := New(Deps{Store: st, Applier: apply.New(st, mirror.New(t.TempDir()), bus.New())})

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST",
		"/api/suggestions/"+sg.ID+"/accept?supersede="+old.ID, nil))
	if rec.Code != 200 {
		t.Fatalf("accept: %d %s", rec.Code, rec.Body.String())
	}
	active, _ := st.ListMemoryEntries(p.ID, false)
	if len(active) != 1 || active[0].Content != "use go build" {
		t.Fatalf("active=%+v", active)
	}
}
