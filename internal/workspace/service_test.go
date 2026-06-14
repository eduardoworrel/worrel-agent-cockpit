package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncProjectDirs(t *testing.T) {
	root := t.TempDir()
	m := New(root)
	real := filepath.Join(t.TempDir(), "repo")
	os.MkdirAll(real, 0o755)

	ws, err := m.SyncProject("escopo-x", []string{real})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "workspaces", "escopo-x")
	if ws != want {
		t.Fatalf("ws=%q", ws)
	}
	if tgt, _ := os.Readlink(filepath.Join(ws, "repo")); tgt != real {
		t.Fatalf("symlink repo → %q", tgt)
	}
}
