package wrapper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/workspace"
)

func TestBuildSpawnOpts(t *testing.T) {
	dir := t.TempDir()
	st := newStore(t)
	st.SetDataDir(dir)
	wm := workspace.New(dir)
	p, _ := st.CreateProject("App", "")
	st.AddProjectDir(p.ID, "/tmp/app")
	st.CreateMemoryEntry(&store.MemoryEntry{ProjectID: p.ID, Content: "Use tabs.", Category: "convencao"})
	sess, _ := st.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})

	opts, err := BuildSpawnOpts(st, wm, sess.ID, 7717, "")
	if err != nil {
		t.Fatal(err)
	}
	if opts.SessionID != sess.ID {
		t.Fatalf("session id = %q", opts.SessionID)
	}
	// cwd deve ser o workspace gerenciado, não /tmp/app diretamente
	if opts.WorkingDir != filepath.Join(dir, "workspaces", p.Slug) {
		t.Fatalf("workdir = %q", opts.WorkingDir)
	}
	if !strings.Contains(opts.Primer, "Use tabs") {
		t.Fatalf("primer sem memória: %q", opts.Primer)
	}
	// SystemAppend deve estar vazio (report tools removidos em sp1)
	if opts.SystemAppend != "" {
		t.Fatalf("system append deve ser vazio, got %q", opts.SystemAppend)
	}
	if !strings.HasPrefix(opts.MCPURL, "http://127.0.0.1:7717/mcp?s=") {
		t.Fatalf("mcp url = %q", opts.MCPURL)
	}
	// token persistido na sessão
	got, _ := st.GetSession(sess.ID)
	if got.MCPToken == nil || *got.MCPToken == "" {
		t.Fatal("mcp_token não persistido")
	}
	if !strings.HasSuffix(opts.MCPURL, *got.MCPToken) {
		t.Fatalf("token na url != token salvo")
	}
}

func TestBuildSpawnOptsWithSkill(t *testing.T) {
	dir := t.TempDir()
	st := newStore(t)
	st.SetDataDir(dir)
	wm := workspace.New(dir)
	p, _ := st.CreateProject("App", "")
	sess, _ := st.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "opencode", Mode: "wrapper"})
	opts, err := BuildSpawnOpts(st, wm, sess.ID, 7717, "# Skill Deploy\nPergunte o ambiente.")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(opts.Primer, "Skill Deploy") {
		t.Fatalf("skill não anexada ao primer: %q", opts.Primer)
	}
}

func TestBuildSpawnOptsUsesWorkspace(t *testing.T) {
	dir := t.TempDir()
	s, _ := store.Open(dir + "/t.db")
	defer s.Close()
	s.SetDataDir(dir)
	p, _ := s.CreateProject("App", "")
	real := filepath.Join(t.TempDir(), "repo")
	os.MkdirAll(real, 0o755)
	s.AddProjectDir(p.ID, real)
	sess, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})

	wm := workspace.New(dir)
	opts, err := BuildSpawnOpts(s, wm, sess.ID, 7717, "")
	if err != nil {
		t.Fatal(err)
	}
	// cwd deve ser o workspace gerenciado, não a pasta real
	if opts.WorkingDir != filepath.Join(dir, "workspaces", p.Slug) {
		t.Fatalf("WorkingDir = %q", opts.WorkingDir)
	}
	// e o symlink para a pasta real existe dentro do workspace
	if _, err := os.Lstat(filepath.Join(opts.WorkingDir, "repo")); err != nil {
		t.Fatalf("symlink não criado: %v", err)
	}
}
