package retro

import (
	"context"
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/apply"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/distill"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/mirror"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// seqCLI: 1ª chamada (clusterização) devolve clusterResp; demais (destilação) candResp.
type seqCLI struct {
	clusterResp string
	candResp    string
	calls       int
}

func (c *seqCLI) RunHeadless(_ context.Context, _ string, _ adapter.HeadlessOpts) (string, error) {
	c.calls++
	if c.calls == 1 {
		return c.clusterResp, nil
	}
	return c.candResp, nil
}

func projectCount(t *testing.T, s *store.Store) int {
	t.Helper()
	ps, err := s.ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	return len(ps)
}

func TestEndToEndIdempotent(t *testing.T) {
	s := newStore(t)
	recent := time.Now().Add(-2 * 24 * time.Hour)
	obs := &fakeObsT{fakeObs{id: "claude-code", sess: []adapter.ExternalSession{
		{Adapter: "claude-code", ExternalRef: "a", Dir: "/repo", Title: "deploy", UpdatedAt: recent},
		{Adapter: "claude-code", ExternalRef: "b", Dir: "/repo", Title: "deploy2", UpdatedAt: recent},
		{Adapter: "claude-code", ExternalRef: "c", Dir: "/repo", Title: "deploy3", UpdatedAt: recent},
	}}}

	run1 := flow(t, s, obs)
	pAfter1 := projectCount(t, s)
	sg1, _ := s.ListSuggestions("", "")

	// 2ª passada: mesmo escopo não deve dobrar projetos nem sugestões
	run2 := flow(t, s, obs)
	if run2.ID != run1.ID {
		// nova run é permitida, mas não deve duplicar artefatos
	}
	pAfter2 := projectCount(t, s)
	sg2, _ := s.ListSuggestions("", "")

	if pAfter2 != pAfter1 {
		t.Fatalf("projetos dobraram: %d -> %d (critério 8)", pAfter1, pAfter2)
	}
	if len(sg2) != len(sg1) {
		t.Fatalf("sugestões dobraram: %d -> %d (critério 8)", len(sg1), len(sg2))
	}
}

// flow roda inventário→plan→cluster→approve→start para o escopo /repo.
func flow(t *testing.T, s *store.Store, obs Observer) *store.RetroRun {
	t.Helper()
	b := bus.New()
	cli := &seqCLI{
		candResp: `[{"type":"skill.learned","title":"Deploy no staging","name":"Deploy",` +
			`"content":"# passos","evidence":"deploy","project_id":""}]`,
	}
	eng := distill.New(s, cli, b)
	applier := apply.New(s, mirror.New(t.TempDir()), b)
	svc := New(s, eng, applier, b, []Observer{obs}, nil)

	_, err := svc.Inventory(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	run, err := svc.Plan(Scope{CLIs: []string{"claude-code"}, Dirs: []string{"/repo"}, WindowDays: 90})
	if err != nil {
		t.Fatal(err)
	}
	// cluster response references the run's pending sessions
	pend, _ := s.PendingRunSessions(run.ID)
	sj := "["
	for i, p := range pend {
		if i > 0 {
			sj += ","
		}
		sj += `"` + p + `"`
	}
	sj += "]"
	cli.clusterResp = `[{"name":"Repo","dirs":["/repo"],"session_ids":` + sj + `,"existing_project_id":""}]`

	if err := svc.Cluster(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	clusters, _ := svc.ListClusters(run.ID)
	for _, c := range clusters {
		if _, err := svc.ApproveCluster(c.ID, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Start(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	return run
}

// fakeObsT seeds transcript content via importer; reuses fakeObs but returns events.
type fakeObsT struct{ fakeObs }

func (f *fakeObsT) ReadTranscript(ref adapter.SessionRef) ([]adapter.TranscriptEvent, error) {
	return []adapter.TranscriptEvent{
		{Role: "user", Kind: "text", Content: "preciso automatizar o deploy"},
		{Role: "assistant", Kind: "text", Content: "houve um erro no build; repetimos os passos de deploy de staging ate ficar verde, vale virar skill"},
	}, nil
}
