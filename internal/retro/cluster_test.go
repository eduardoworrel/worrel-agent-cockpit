package retro

import (
	"context"
	"strings"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

type scriptCLI struct {
	calls int
	resp  string
}

func (c *scriptCLI) RunHeadless(_ context.Context, _ string, _ adapter.HeadlessOpts) (string, error) {
	c.calls++
	return c.resp, nil
}

func seedClusterRun(t *testing.T, s *store.Store) (*store.RetroRun, string) {
	t.Helper()
	p, _ := s.CreateProject("App", "")
	run, _ := s.CreateRetroRun(&store.RetroRun{Status: "clustered"})
	se, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "observed"})
	_ = s.EndSession(se.ID)
	_ = s.AddRunSession(run.ID, se.ID, "")
	return run, se.ID
}

func TestClustererProposes(t *testing.T) {
	s := newStore(t)
	run, sid := seedClusterRun(t, s)
	cli := &scriptCLI{resp: `[{"name":"API","dirs":["/a","/b"],"session_ids":["` + sid + `"],"existing_project_id":""}]`}
	cl := NewClusterer(s, cli, bus.New())
	if err := cl.Propose(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListRetroClusters(run.ID)
	if len(list) != 1 || list[0].Name != "API" {
		t.Fatalf("clusters %+v", list)
	}
	if cli.calls != 1 {
		t.Fatalf("llm calls = %d, want 1", cli.calls)
	}
	got, _ := s.GetRetroRun(run.ID)
	if got.LLMCalls != 1 {
		t.Fatalf("run.LLMCalls = %d, want 1", got.LLMCalls)
	}
}

// TestClustererGroupsBySourceDir verifies that unclassified sessions (empty project_id)
// are grouped by their SourceDir rather than all collapsing into a single "" bucket.
func TestClustererGroupsBySourceDir(t *testing.T) {
	s := newStore(t)
	run, _ := s.CreateRetroRun(&store.RetroRun{Status: "inventoried"})

	makeSession := func(sourceDir string) string {
		sess, err := s.CreateSession(&store.Session{
			// no ProjectID → unclassified
			Adapter:   "claude-code",
			Mode:      "observed",
			SourceDir: sourceDir,
		})
		if err != nil {
			t.Fatal(err)
		}
		_ = s.EndSession(sess.ID)
		_ = s.AddRunSession(run.ID, sess.ID, "")
		return sess.ID
	}

	// 3 sessions in /repos/a, 2 in /repos/b
	makeSession("/repos/a")
	makeSession("/repos/a")
	makeSession("/repos/a")
	makeSession("/repos/b")
	makeSession("/repos/b")

	// Capture the byDir map by inspecting what the Clusterer sends to the LLM.
	var capturedPrompt string
	cli := &scriptCLI{resp: `[]`}
	cli.resp = `[]`
	capturingCLI := &captureCLI{inner: cli, prompt: &capturedPrompt}

	cl := NewClusterer(s, capturingCLI, bus.New())
	if err := cl.Propose(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}

	// The prompt must mention both folders, not just an empty dir.
	if !strings.Contains(capturedPrompt, "/repos/a") {
		t.Error("prompt missing /repos/a")
	}
	if !strings.Contains(capturedPrompt, "/repos/b") {
		t.Error("prompt missing /repos/b")
	}
	if strings.Contains(capturedPrompt, `pasta="" `) || strings.Contains(capturedPrompt, "pasta=\"\"") {
		t.Error("prompt contains empty-string dir group — unclassified sessions were not grouped by SourceDir")
	}

	// Also verify that GetSession correctly returns SourceDir.
	all, _ := s.ListSessions("")
	for _, sess := range all {
		if sess.SourceDir != "/repos/a" && sess.SourceDir != "/repos/b" {
			t.Errorf("unexpected SourceDir %q for session %s", sess.SourceDir, sess.ID)
		}
	}
}

type captureCLI struct {
	inner  *scriptCLI
	prompt *string
}

func (c *captureCLI) RunHeadless(ctx context.Context, prompt string, opts adapter.HeadlessOpts) (string, error) {
	*c.prompt = prompt
	return c.inner.RunHeadless(ctx, prompt, opts)
}

// TestClustererAssignsSessionsFromDirs: o LLM responde só com PASTAS (sem
// session_ids) e as sessões são atribuídas localmente pela pasta — o prompt NÃO
// carrega IDs de sessão (escala O(pastas), não O(sessões)).
func TestClustererAssignsSessionsFromDirs(t *testing.T) {
	s := newStore(t)
	run, _ := s.CreateRetroRun(&store.RetroRun{Status: "inventoried"})
	mk := func(dir string) string {
		sess, _ := s.CreateSession(&store.Session{Adapter: "claude-code", Mode: "observed", SourceDir: dir, Title: "t-" + dir})
		_ = s.EndSession(sess.ID)
		_ = s.AddRunSession(run.ID, sess.ID, "")
		return sess.ID
	}
	a1, a2, b1 := mk("/repos/a"), mk("/repos/a"), mk("/repos/b")

	var prompt string
	inner := &scriptCLI{resp: `[{"name":"A","dirs":["/repos/a"],"existing_project_id":""},` +
		`{"name":"B","dirs":["/repos/b"],"existing_project_id":""}]`}
	cl := NewClusterer(s, &captureCLI{inner: inner, prompt: &prompt}, bus.New())
	if err := cl.Propose(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}

	// O prompt não deve conter NENHUM id de sessão (resumo por pasta).
	for _, id := range []string{a1, a2, b1} {
		if strings.Contains(prompt, id) {
			t.Fatalf("prompt vazou id de sessão %s (deveria ser resumo por pasta)", id)
		}
	}
	// Sessões atribuídas localmente: cluster A tem 2, cluster B tem 1.
	list, _ := s.ListRetroClusters(run.ID)
	byName := map[string]string{}
	for _, c := range list {
		byName[c.Name] = c.SessionIDs
	}
	if !strings.Contains(byName["A"], a1) || !strings.Contains(byName["A"], a2) {
		t.Fatalf("cluster A deveria conter as 2 sessões de /repos/a: %q", byName["A"])
	}
	if !strings.Contains(byName["B"], b1) || strings.Contains(byName["B"], a1) {
		t.Fatalf("cluster B deveria conter só a sessão de /repos/b: %q", byName["B"])
	}
}

func TestClustererAssociatesExisting(t *testing.T) {
	s := newStore(t)
	run, sid := seedClusterRun(t, s)
	ep, _ := s.CreateProject("Existente", "")
	cli := &scriptCLI{resp: `[{"name":"X","dirs":[],"session_ids":["` + sid + `"],"existing_project_id":"` + ep.ID + `"}]`}
	cl := NewClusterer(s, cli, bus.New())
	if err := cl.Propose(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListRetroClusters(run.ID)
	if len(list) != 1 || list[0].ExistingProjectID == nil || *list[0].ExistingProjectID != ep.ID {
		t.Fatalf("associação ausente: %+v", list[0])
	}
}
