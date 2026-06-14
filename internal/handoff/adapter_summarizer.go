package handoff

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
)

// adapterSummarizer usa o headless do adaptador como Summarizer.
type adapterSummarizer struct {
	ad adapter.Adapter
}

// NewAdapterSummarizer cria um Summarizer que delega a RunHeadless do
// adaptador e reduz a saída ao texto final do assistente — sessions.summary
// guarda só o markdown limpo das 6 seções, nunca o envelope JSON do CLI.
func NewAdapterSummarizer(ad adapter.Adapter) Summarizer { return &adapterSummarizer{ad: ad} }

func (a *adapterSummarizer) Summarize(ctx context.Context, prompt string) (string, error) {
	out, err := a.ad.RunHeadless(ctx, prompt, adapter.HeadlessOpts{})
	if err != nil {
		return "", err
	}
	return extractResultText(out), nil
}

// extractResultText extrai o texto final do assistente da saída headless.
// Formatos aceitos:
//   - array de eventos (claude --output-format json/stream-json): pega o
//     último evento {"type":"result","result":"..."};
//   - objeto único com campo "result" string;
//   - texto puro (passthrough).
func extractResultText(out string) string {
	s := strings.TrimSpace(out)
	switch {
	case strings.HasPrefix(s, "["):
		var events []map[string]any
		if err := json.Unmarshal([]byte(s), &events); err == nil {
			for i := len(events) - 1; i >= 0; i-- {
				if events[i]["type"] == "result" {
					if r, ok := events[i]["result"].(string); ok && r != "" {
						return strings.TrimSpace(r)
					}
				}
			}
		}
	case strings.HasPrefix(s, "{"):
		var obj map[string]any
		if err := json.Unmarshal([]byte(s), &obj); err == nil {
			if r, ok := obj["result"].(string); ok && r != "" {
				return strings.TrimSpace(r)
			}
		}
	}
	return s
}
