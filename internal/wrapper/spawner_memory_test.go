package wrapper

import (
	"strings"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/workspace"
)

func TestBuildSpawnInjectsRenderedMemory(t *testing.T) {
	dir := t.TempDir()
	st := newStore(t)
	st.SetDataDir(dir)
	wm := workspace.New(dir)
	p, _ := st.CreateProject("App", "")
	_, _ = st.CreateMemoryEntry(&store.MemoryEntry{ProjectID: p.ID, Content: "use go build", Category: "convencao"})
	sess, _ := st.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})

	// default delivery=always_inject → injeta o render
	opts, err := BuildSpawnOpts(st, wm, sess.ID, 8080, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(opts.Primer, "use go build") {
		t.Fatalf("primer sem memória renderizada: %q", opts.Primer)
	}

	// on_demand → não injeta
	_ = st.SetEngineConfig("memory", "delivery", "on_demand", p.ID)
	opts, _ = BuildSpawnOpts(st, wm, sess.ID, 8080, "", "")
	if strings.Contains(opts.Primer, "use go build") {
		t.Fatalf("on_demand não deveria injetar memória: %q", opts.Primer)
	}
}
