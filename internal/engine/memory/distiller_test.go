package memory

import (
	"context"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

type fakeHeadless struct {
	prompt string
	out    string
}

func (f *fakeHeadless) RunHeadless(_ context.Context, prompt string, _ adapter.HeadlessOpts) (string, error) {
	f.prompt = prompt
	return f.out, nil
}

func TestParseGoldenTruths(t *testing.T) {
	raw := "lixo antes [{\"content\":\"use go build\",\"category\":\"convencao\",\"evidence\":\"sess1\",\"related_entry_id\":\"\"}] lixo depois"
	gts := parseGoldenTruths(raw)
	if len(gts) != 1 || gts[0].Content != "use go build" || gts[0].Category != "convencao" {
		t.Fatalf("parse=%+v", gts)
	}
}

func TestDistillCallsHeadlessAndParses(t *testing.T) {
	fh := &fakeHeadless{out: `[{"content":"build é go build ./...","category":"convencao","evidence":"s1"}]`}
	d := NewLLMDistiller(fh, "PROMPT_BASE")
	windows := []FrictionWindow{{Signal: "error_then_success", Events: []*store.TranscriptEvent{
		ev(1, "assistant", "tool_use", "Bash make build", ""),
	}}}
	gts, err := d.Distill(context.Background(), windows, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(gts) != 1 || gts[0].Category != "convencao" {
		t.Fatalf("gts=%+v", gts)
	}
	if fh.prompt == "PROMPT_BASE" || len(fh.prompt) <= len("PROMPT_BASE") {
		t.Fatalf("prompt deveria incluir o conteúdo das janelas, got %q", fh.prompt)
	}
}
