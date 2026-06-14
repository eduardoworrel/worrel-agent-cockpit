package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
)

// baseFakeAdapter implementa adapter.Adapter (sem ModelLister).
type baseFakeAdapter struct{ id string }

func (f baseFakeAdapter) ID() string { return f.id }
func (f baseFakeAdapter) Detect() (adapter.Installed, error) {
	return adapter.Installed{Present: true}, nil
}
func (f baseFakeAdapter) Capabilities() adapter.Caps { return adapter.Caps{} }
func (f baseFakeAdapter) BuildInteractive(adapter.SpawnOpts) (adapter.CmdSpec, error) {
	return adapter.CmdSpec{}, nil
}
func (f baseFakeAdapter) RunHeadless(context.Context, string, adapter.HeadlessOpts) (string, error) {
	return "", nil
}
func (f baseFakeAdapter) DiscoverSessions(time.Time) ([]adapter.ExternalSession, error) {
	return nil, nil
}
func (f baseFakeAdapter) ReadTranscript(adapter.SessionRef) ([]adapter.TranscriptEvent, error) {
	return nil, nil
}
func (f baseFakeAdapter) ContextUsage(adapter.SessionRef) (int, int, bool) { return 0, 0, false }

// listerFakeAdapter adiciona ModelLister.
type listerFakeAdapter struct {
	baseFakeAdapter
	models []string
	err    error
}

func (f listerFakeAdapter) ListModels(context.Context) ([]string, error) {
	return f.models, f.err
}

func newModelsServer(t *testing.T, adapters ...adapter.Adapter) *httptest.Server {
	t.Helper()
	reg := adapter.NewRegistry()
	for _, a := range adapters {
		reg.Register(a)
	}
	srv := New(Deps{Adapters: reg})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func getModels(t *testing.T, ts *httptest.Server, id string) (int, []string) {
	t.Helper()
	resp, err := ts.Client().Get(ts.URL + "/api/adapters/" + id + "/models")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body modelsResponse
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body.Models
}

func TestModelsListerReturnsList(t *testing.T) {
	want := []string{"provider/a", "provider/b"}
	ts := newModelsServer(t, listerFakeAdapter{
		baseFakeAdapter: baseFakeAdapter{id: "fake"},
		models:          want,
	})
	code, got := getModels(t, ts, "fake")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if len(got) != 2 || got[0] != "provider/a" || got[1] != "provider/b" {
		t.Fatalf("models = %v, want %v", got, want)
	}
}

func TestModelsWithoutListerReturnsEmpty(t *testing.T) {
	ts := newModelsServer(t, baseFakeAdapter{id: "plain"})
	code, got := getModels(t, ts, "plain")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if len(got) != 0 {
		t.Fatalf("models = %v, want []", got)
	}
}

func TestModelsUnknownAdapter404(t *testing.T) {
	ts := newModelsServer(t, baseFakeAdapter{id: "plain"})
	code, _ := getModels(t, ts, "nope")
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", code)
	}
}

func TestModelsListerErrorDegrades502(t *testing.T) {
	ts := newModelsServer(t, listerFakeAdapter{
		baseFakeAdapter: baseFakeAdapter{id: "broken"},
		err:             errors.New("cli não instalado"),
	})
	code, got := getModels(t, ts, "broken")
	if code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", code)
	}
	if len(got) != 0 {
		t.Fatalf("models = %v, want []", got)
	}
}
