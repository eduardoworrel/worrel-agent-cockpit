package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureAndSyncSymlinks(t *testing.T) {
	root := t.TempDir()
	m := New(root)

	ws, err := m.EnsureWorkspace("meu-escopo")
	if err != nil {
		t.Fatal(err)
	}
	if ws != filepath.Join(root, "workspaces", "meu-escopo") {
		t.Fatalf("ws = %q", ws)
	}
	if fi, err := os.Stat(ws); err != nil || !fi.IsDir() {
		t.Fatalf("workspace não criado: %v", err)
	}
	// idempotente
	if _, err := m.EnsureWorkspace("meu-escopo"); err != nil {
		t.Fatalf("EnsureWorkspace não idempotente: %v", err)
	}

	// duas pastas reais com mesmo basename → sufixo
	a := filepath.Join(t.TempDir(), "api")
	b := filepath.Join(t.TempDir(), "api")
	os.MkdirAll(a, 0o755)
	os.MkdirAll(b, 0o755)
	if err := m.SyncSymlinks(ws, []string{a, b}); err != nil {
		t.Fatal(err)
	}
	if tgt, _ := os.Readlink(filepath.Join(ws, "api")); tgt != a {
		t.Fatalf("symlink api → %q, want %q", tgt, a)
	}
	if tgt, _ := os.Readlink(filepath.Join(ws, "api-2")); tgt != b {
		t.Fatalf("symlink api-2 → %q, want %q", tgt, b)
	}

	// sync removendo b: api-2 some, api fica
	if err := m.SyncSymlinks(ws, []string{a}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(ws, "api-2")); !os.IsNotExist(err) {
		t.Fatal("api-2 deveria ter sido removido")
	}
	if _, err := os.Lstat(filepath.Join(ws, "api")); err != nil {
		t.Fatal("api deveria permanecer")
	}
}

func TestBrokenTargetAndScratch(t *testing.T) {
	root := t.TempDir()
	m := New(root)
	ws, _ := m.EnsureWorkspace("x")
	// alvo inexistente: symlink é criado mesmo assim; Broken() reporta
	missing := filepath.Join(t.TempDir(), "sumiu")
	if err := m.SyncSymlinks(ws, []string{missing}); err != nil {
		t.Fatal(err)
	}
	broken := m.BrokenLinks(ws)
	if len(broken) != 1 || broken[0] != "sumiu" {
		t.Fatalf("broken = %v", broken)
	}
	// scratch
	sc, err := m.ScratchWorkspace("sess-123")
	if err != nil {
		t.Fatal(err)
	}
	if sc != filepath.Join(root, "workspaces", "_scratch-sess-123") {
		t.Fatalf("scratch = %q", sc)
	}
	if fi, _ := os.Stat(sc); fi == nil || !fi.IsDir() {
		t.Fatal("scratch não criado")
	}
}
