package retro

import (
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/distill"
)

func countSessions(t *testing.T, p *Planner) int {
	t.Helper()
	var n int
	if err := p.store.DB().QueryRow(`SELECT count(*) FROM sessions`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPlannerImportsScopeAndIdempotent(t *testing.T) {
	s := newStore(t)
	old := time.Now().Add(-100 * 24 * time.Hour)
	recent := time.Now().Add(-5 * 24 * time.Hour)
	obs := &fakeObs{id: "claude-code", sess: []adapter.ExternalSession{
		{Adapter: "claude-code", ExternalRef: "a", Dir: "/x", UpdatedAt: old},
		{Adapter: "claude-code", ExternalRef: "b", Dir: "/x", UpdatedAt: recent},
		{Adapter: "claude-code", ExternalRef: "c", Dir: "/y", UpdatedAt: recent},
	}}
	b := bus.New()
	imp := distill.NewImporter(s, b)
	p := NewPlanner(s, imp, []Observer{obs})

	run, err := p.Plan(Scope{CLIs: []string{"claude-code"}, Dirs: []string{"/x"}, WindowDays: 60})
	if err != nil {
		t.Fatal(err)
	}
	pend, _ := s.PendingRunSessions(run.ID)
	// só 'b' está em /x e dentro de 60d
	if len(pend) != 1 {
		t.Fatalf("pendentes = %d, want 1", len(pend))
	}
	n1 := countSessions(t, p)

	// idempotência: replanejar o mesmo escopo não cria sessões nem run-sessions duplicadas
	run2, err := p.Plan(Scope{CLIs: []string{"claude-code"}, Dirs: []string{"/x"}, WindowDays: 60})
	if err != nil {
		t.Fatal(err)
	}
	if n2 := countSessions(t, p); n2 != n1 {
		t.Fatalf("sessões dobraram: %d -> %d", n1, n2)
	}
	pend2, _ := s.PendingRunSessions(run2.ID)
	if len(pend2) != 1 {
		t.Fatalf("pendentes run2 = %d, want 1", len(pend2))
	}
}
