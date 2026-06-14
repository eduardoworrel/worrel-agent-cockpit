package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/apply"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/distill"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/mirror"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/retro"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

type retroFakeObs struct {
	sess []adapter.ExternalSession
}

func (f *retroFakeObs) ID() string { return "claude-code" }
func (f *retroFakeObs) DiscoverSessions(since time.Time) ([]adapter.ExternalSession, error) {
	var out []adapter.ExternalSession
	for _, s := range f.sess {
		if since.IsZero() || s.UpdatedAt.After(since) {
			out = append(out, s)
		}
	}
	return out, nil
}
func (f *retroFakeObs) ReadTranscript(_ adapter.SessionRef) ([]adapter.TranscriptEvent, error) {
	return []adapter.TranscriptEvent{
		{Role: "user", Kind: "text", Content: "preciso automatizar o deploy"},
		{Role: "assistant", Kind: "text", Content: "houve um erro no build; repetimos os passos de deploy de staging ate ficar verde, vale virar skill"},
	}, nil
}

type retroCLI struct{ resp string }

func (c *retroCLI) RunHeadless(_ context.Context, _ string, _ adapter.HeadlessOpts) (string, error) {
	return c.resp, nil
}

func newRetroServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	b := bus.New()
	cli := &retroCLI{resp: "[]"}
	eng := distill.New(s, cli, b)
	applier := apply.New(s, mirror.New(t.TempDir()), b)
	obs := &retroFakeObs{sess: []adapter.ExternalSession{
		{Adapter: "claude-code", ExternalRef: "a", Dir: "/repo", Title: "t", UpdatedAt: time.Now().Add(-2 * 24 * time.Hour)},
	}}
	svc := retro.New(s, eng, applier, b, []retro.Observer{obs}, nil)
	srv := New(Deps{Store: s, Bus: b, Applier: applier, Retro: svc})
	return httptest.NewServer(srv.Handler()), s
}

func TestRetroInventoryAndRunLifecycle(t *testing.T) {
	ts, _ := newRetroServer(t)
	defer ts.Close()
	c := ts.Client()

	// inventário sem LLM
	resp, err := c.Post(ts.URL+"/api/retro/inventory", "application/json", strings.NewReader(`{}`))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("inventory status: %v %d", err, resp.StatusCode)
	}
	var rep retro.InventoryReport
	_ = json.NewDecoder(resp.Body).Decode(&rep)
	if rep.PerCLI["claude-code"] == nil || rep.PerCLI["claude-code"].Sessions != 1 {
		t.Fatalf("inventário inesperado: %+v", rep.PerCLI)
	}

	// abre run
	body := `{"scope":{"clis":["claude-code"],"dirs":["/repo"],"window_days":90},"depth":"completa","budget_per_hour":0}`
	resp, err = c.Post(ts.URL+"/api/retro/runs", "application/json", strings.NewReader(body))
	if err != nil || resp.StatusCode != 201 {
		t.Fatalf("runs status: %v %d", err, resp.StatusCode)
	}
	var run store.RetroRun
	_ = json.NewDecoder(resp.Body).Decode(&run)
	if run.ID == "" {
		t.Fatal("run sem id")
	}

	// cluster (1ª LLM) — resp vazio gera nenhum cluster, mas avança status
	resp, _ = c.Post(ts.URL+"/api/retro/runs/"+run.ID+"/cluster", "application/json", strings.NewReader(`{}`))
	if resp.StatusCode != 200 {
		t.Fatalf("cluster status %d", resp.StatusCode)
	}

	// batch view (vazio) ok
	resp, _ = c.Get(ts.URL + "/api/retro/runs/" + run.ID + "/batch")
	if resp.StatusCode != 200 {
		t.Fatalf("batch status %d", resp.StatusCode)
	}
}

func TestRetroSecretRejectSuppresses(t *testing.T) {
	ts, s := newRetroServer(t)
	defer ts.Close()
	c := ts.Client()

	p, _ := s.CreateProject("App", "")
	sc := retro.NewSecretScan(s)
	raw := "ghp_0123456789abcdefghij"
	n, _ := sc.Scan(p.ID, []string{"token=" + raw})
	if n != 1 {
		t.Fatalf("scan inicial = %d", n)
	}
	sg, _ := s.ListSuggestions(p.ID, "pending")
	var secretID string
	for _, x := range sg {
		if x.Type == "secret.detected" {
			secretID = x.ID
		}
	}
	if secretID == "" {
		t.Fatal("sugestão secret ausente")
	}

	// reject via API → deve suprimir o hash
	resp, err := c.Post(ts.URL+"/api/suggestions/"+secretID+"/reject", "application/json", strings.NewReader(`{}`))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("reject status: %v %d", err, resp.StatusCode)
	}

	// segundo scan não re-sugere
	n2, _ := sc.Scan(p.ID, []string{"token=" + raw})
	if n2 != 0 {
		t.Fatalf("após reject = %d, want 0 (critério 9)", n2)
	}
}
