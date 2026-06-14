package adapter

import (
	"context"
	"testing"
	"time"
)

type fakeAdapter struct {
	id        string
	installed bool
}

func (f fakeAdapter) ID() string                   { return f.id }
func (f fakeAdapter) Detect() (Installed, error)   { return Installed{Present: f.installed, Version: "9.9"}, nil }
func (f fakeAdapter) Capabilities() Caps           { return Caps{Headless: true} }
func (f fakeAdapter) BuildInteractive(SpawnOpts) (CmdSpec, error) {
	return CmdSpec{Path: "/bin/true"}, nil
}
func (f fakeAdapter) RunHeadless(ctx context.Context, prompt string, _ HeadlessOpts) (string, error) {
	return prompt, nil
}
func (f fakeAdapter) DiscoverSessions(since time.Time) ([]ExternalSession, error) {
	return nil, ErrNotSupported
}
func (f fakeAdapter) ReadTranscript(SessionRef) ([]TranscriptEvent, error) {
	return nil, ErrNotSupported
}
func (f fakeAdapter) ContextUsage(ref SessionRef) (used, limit int, ok bool) { return 0, 0, false }

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	r.Register(fakeAdapter{id: "a", installed: true})
	r.Register(fakeAdapter{id: "b", installed: false})

	if _, ok := r.Get("a"); !ok {
		t.Fatal("Get(a) deve existir")
	}
	if _, ok := r.Get("nope"); ok {
		t.Fatal("Get(nope) não deve existir")
	}
	det := r.Detected()
	if len(det) != 2 {
		t.Fatalf("Detected() = %d, want 2", len(det))
	}
	// ordenado por ID e carregando Present
	if det[0].ID != "a" || !det[0].Installed.Present {
		t.Fatalf("det[0] = %+v", det[0])
	}
	if det[1].ID != "b" || det[1].Installed.Present {
		t.Fatalf("det[1] = %+v", det[1])
	}
}
