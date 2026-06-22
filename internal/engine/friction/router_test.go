package friction

import (
	"context"
	"strings"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
)

type fakeHeadless struct{ out, gotPrompt string }

func (f *fakeHeadless) RunHeadless(_ context.Context, p string, _ adapter.HeadlessOpts) (string, error) {
	f.gotPrompt = p
	return f.out, nil
}

func TestParseDecisions(t *testing.T) {
	raw := `lixo [{"destino":"memory","memory":{"content":"use go build","category":"convencao"}}] fim`
	ds := parseDecisions(raw)
	if len(ds) != 1 || ds[0].Destino != "memory" || ds[0].Memory.Content == "" {
		t.Fatalf("parse=%+v", ds)
	}
}

func TestRouteBuildsPromptAndParses(t *testing.T) {
	fh := &fakeHeadless{out: `[{"destino":"refine_skill","skill":{"skill_id":"s1","content":"novo","change_summary":"fix"}}]`}
	r := NewRouter(fh, "")
	ds, err := r.Route(context.Background(), []Signal{{Kind: "error_then_success", Text: "make falhou; go build ok"}}, "CTX")
	if err != nil || len(ds) != 1 || ds[0].Destino != "refine_skill" {
		t.Fatalf("route: %v %+v", err, ds)
	}
	if !strings.Contains(fh.gotPrompt, "CTX") || !strings.Contains(fh.gotPrompt, "make falhou") {
		t.Fatalf("prompt incompleto: %q", fh.gotPrompt)
	}
}
