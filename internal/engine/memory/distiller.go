package memory

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// Headless é a dependência mínima de LLM one-shot (satisfeita por adapter.Adapter).
type Headless interface {
	RunHeadless(ctx context.Context, prompt string, opts adapter.HeadlessOpts) (string, error)
}

// GoldenTruth é um fato anti-erro destilado, pronto para virar memory_entry.
type GoldenTruth struct {
	Content        string `json:"content"`
	Category       string `json:"category"`
	Evidence       string `json:"evidence"`
	RelatedEntryID string `json:"related_entry_id"`
}

type LLMDistiller struct {
	h      Headless
	prompt string
}

func NewLLMDistiller(h Headless, prompt string) *LLMDistiller {
	return &LLMDistiller{h: h, prompt: prompt}
}

// Distill monta o prompt (base + entradas atuais + janelas) e parseia a saída.
func (d *LLMDistiller) Distill(ctx context.Context, windows []FrictionWindow, currentEntries []*store.MemoryEntry) ([]GoldenTruth, error) {
	var b strings.Builder
	b.WriteString(d.prompt)
	b.WriteString("\n\n## Memória atual (não repita, sinalize conflito por id)\n")
	for _, e := range currentEntries {
		b.WriteString("- [" + e.ID + "] (" + e.Category + ") " + e.Content + "\n")
	}
	b.WriteString("\n## Trechos de atrito da sessão\n")
	for _, w := range windows {
		b.WriteString("### sinal: " + w.Signal + "\n")
		for _, ev := range w.Events {
			b.WriteString(ev.Role + "/" + ev.Kind + ": " + ev.Content + "\n")
		}
	}
	raw, err := d.h.RunHeadless(ctx, b.String(), adapter.HeadlessOpts{})
	if err != nil {
		return nil, err
	}
	return parseGoldenTruths(raw), nil
}

// parseGoldenTruths extrai o primeiro array JSON de GoldenTruth da saída crua do
// LLM (tolerante a texto em volta).
func parseGoldenTruths(raw string) []GoldenTruth {
	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start < 0 || end <= start {
		return nil
	}
	var gts []GoldenTruth
	if json.Unmarshal([]byte(raw[start:end+1]), &gts) != nil {
		return nil
	}
	return gts
}
