package skill

import (
	"context"
	"strings"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

type fakeHeadless struct{ out, gotPrompt string }

func (f *fakeHeadless) RunHeadless(_ context.Context, p string, _ adapter.HeadlessOpts) (string, error) {
	f.gotPrompt = p
	return f.out, nil
}

func TestParseDrafts(t *testing.T) {
	raw := `ruído [{"signature":"sig-deploy","title":"Deploy","skill_draft":{"name":"Deploy","content":"## passos","structured":"{}"},"agente_draft":{"name":"Deployer","persona":"Você cuida de deploys."}}] fim`
	ds := parseDrafts(raw)
	if len(ds) != 1 || ds[0].Signature != "sig-deploy" || ds[0].AgenteDraft.Persona == "" {
		t.Fatalf("parse=%+v", ds)
	}
}

func TestDistillBuildsPromptAndParses(t *testing.T) {
	fh := &fakeHeadless{out: `[{"signature":"s1","title":"T","skill_draft":{"name":"T","content":"c","structured":"{}"},"agente_draft":{"name":"A","persona":"p"}}]`}
	d := NewDistiller(fh, "")
	w := []WorkflowWindow{{Signal: "user_steps", Events: []*store.TranscriptEvent{ev(1, "user", "text", "primeiro X depois Y", "")}}}
	ds, err := d.Distill(context.Background(), w, nil, "SKILL_PROMPT", "AGENT_PROMPT")
	if err != nil || len(ds) != 1 {
		t.Fatalf("distill: %v %+v", err, ds)
	}
	if !strings.Contains(fh.gotPrompt, "SKILL_PROMPT") || !strings.Contains(fh.gotPrompt, "AGENT_PROMPT") || !strings.Contains(fh.gotPrompt, "primeiro X depois Y") {
		t.Fatalf("prompt incompleto: %q", fh.gotPrompt)
	}
}
