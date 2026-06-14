package retro

import (
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

type fakeObs struct {
	id   string
	sess []adapter.ExternalSession
}

func (f *fakeObs) ID() string { return f.id }
func (f *fakeObs) DiscoverSessions(since time.Time) ([]adapter.ExternalSession, error) {
	var out []adapter.ExternalSession
	for _, s := range f.sess {
		if since.IsZero() || s.UpdatedAt.After(since) {
			out = append(out, s)
		}
	}
	return out, nil
}
func (f *fakeObs) ReadTranscript(ref adapter.SessionRef) ([]adapter.TranscriptEvent, error) {
	return nil, nil
}

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestInventoryNoLLMAndWindow(t *testing.T) {
	s := newStore(t)
	old := time.Now().Add(-100 * 24 * time.Hour)
	recent := time.Now().Add(-10 * 24 * time.Hour)
	obs := &fakeObs{id: "claude-code", sess: []adapter.ExternalSession{
		{Adapter: "claude-code", ExternalRef: "a", Dir: "/repo/x", UpdatedAt: old},
		{Adapter: "claude-code", ExternalRef: "b", Dir: "/repo/x", UpdatedAt: recent},
		{Adapter: "claude-code", ExternalRef: "c", Dir: "/repo/y", UpdatedAt: recent},
	}}
	inv := NewInventory(s, []Observer{obs})

	full, err := inv.Scan(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if full.PerCLI["claude-code"].Sessions != 3 {
		t.Fatalf("total = %d", full.PerCLI["claude-code"].Sessions)
	}
	if len(full.Folders) != 2 {
		t.Fatalf("pastas = %d", len(full.Folders))
	}
	if full.EstimatedInvocations != full.PerCLI["claude-code"].Sessions {
		t.Fatalf("estimativa != sessões: %d", full.EstimatedInvocations)
	}
	// critério 2: janela de 60d reduz proporcionalmente (só 'b' e 'c' são recentes)
	win, _ := inv.Scan(time.Now().Add(-60 * 24 * time.Hour))
	if win.PerCLI["claude-code"].Sessions != 2 {
		t.Fatalf("janela 60d = %d, want 2", win.PerCLI["claude-code"].Sessions)
	}
}

func TestInventoryMarksKnown(t *testing.T) {
	s := newStore(t)
	ref := "a"
	_, _ = s.CreateSession(&store.Session{Adapter: "claude-code", Mode: "observed", ExternalRef: &ref})
	obs := &fakeObs{id: "claude-code", sess: []adapter.ExternalSession{
		{Adapter: "claude-code", ExternalRef: "a", Dir: "/x", UpdatedAt: time.Now()},
		{Adapter: "claude-code", ExternalRef: "b", Dir: "/x", UpdatedAt: time.Now()},
	}}
	inv := NewInventory(s, []Observer{obs})
	r, _ := inv.Scan(time.Time{})
	if r.PerCLI["claude-code"].AlreadyKnown != 1 {
		t.Fatalf("known = %d, want 1", r.PerCLI["claude-code"].AlreadyKnown)
	}
	if r.EstimatedInvocations != 1 {
		t.Fatalf("estimativa escopo padrão = %d, want 1", r.EstimatedInvocations)
	}
}
