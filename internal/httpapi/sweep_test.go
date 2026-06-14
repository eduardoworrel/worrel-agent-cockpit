package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/distill"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

type noopHeadless struct{}

func (noopHeadless) RunHeadless(_ context.Context, _ string, _ adapter.HeadlessOpts) (string, error) {
	return "[]", nil
}

func TestSweepEndpoint(t *testing.T) {
	s, _ := store.Open(t.TempDir() + "/t.db")
	t.Cleanup(func() { s.Close() })
	eng := distill.New(s, noopHeadless{}, bus.New())
	srv := New(Deps{Store: s, Distiller: eng})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	resp, err := ts.Client().Post(ts.URL+"/api/sweep", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
