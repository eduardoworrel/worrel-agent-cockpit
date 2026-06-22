package wrapper

import (
	"strings"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/workspace"
)

func TestBuildSpawnOptsAgentPersona(t *testing.T) {
	dir := t.TempDir()
	st := newStore(t)
	st.SetDataDir(dir)
	wm := workspace.New(dir)

	sess, _ := st.CreateSession(&store.Session{Adapter: "claude-code", Mode: "wrapper"})

	persona := "Você é um revisor Go rigoroso."
	opts, err := BuildSpawnOpts(st, wm, sess.ID, 8080, "", persona)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(opts.SystemAppend, persona) {
		t.Fatalf("SystemAppend deveria conter a persona, got: %q", opts.SystemAppend)
	}

	// skill still in Primer
	skillContent := "## minha skill"
	opts2, err := BuildSpawnOpts(st, wm, sess.ID, 8080, skillContent, persona)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(opts2.Primer, skillContent) {
		t.Fatalf("Primer deveria conter o skill content, got: %q", opts2.Primer)
	}
	if !strings.Contains(opts2.SystemAppend, persona) {
		t.Fatalf("SystemAppend deveria conter a persona, got: %q", opts2.SystemAppend)
	}

	// no persona → empty SystemAppend
	opts3, err := BuildSpawnOpts(st, wm, sess.ID, 8080, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if opts3.SystemAppend != "" {
		t.Fatalf("SystemAppend deveria ser vazio, got: %q", opts3.SystemAppend)
	}
}
