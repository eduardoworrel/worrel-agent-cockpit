// Package handoff gera resumos estruturados de sessão (spec §9) e encadeia
// sessões quando o contexto se esgota. O resumo é produzido por um Summarizer
// (na fase 3, o adaptador headless do CLI preferido) sobre o transcript
// normalizado, e persistido em sessions.summary.
package handoff

import (
	"context"
	"fmt"
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// Summarizer roda um prompt headless e devolve texto. A fase 3 conecta isto
// ao adaptador (adapter.RunHeadless). Em teste, é fakeado.
type Summarizer interface {
	Summarize(ctx context.Context, prompt string) (string, error)
}

type Generator struct {
	store      *store.Store
	summarizer Summarizer
}

func New(s *store.Store, sum Summarizer) *Generator {
	return &Generator{store: s, summarizer: sum}
}

// PromptHeader é o cabeçalho fixo com as 6 seções obrigatórias (spec §9).
const PromptHeader = `Você está gerando um RESUMO DE HANDOFF para continuar este trabalho em uma nova sessão.
Produza Markdown com EXATAMENTE estas seções, nesta ordem, preenchidas a partir do transcript abaixo:

## Estado atual
## O que foi feito
## Decisões
## Próxima ação
## Não repetir
## Arquivos relevantes

Seja conciso e factual. Não invente. Liste em "Não repetir" os caminhos que falharam.

--- TRANSCRIPT ---
`

// GenerateSummary monta o prompt, chama o summarizer e persiste o resultado.
func (g *Generator) GenerateSummary(ctx context.Context, sessionID string) (string, error) {
	events, err := g.store.ListTranscriptEvents(sessionID)
	if err != nil {
		return "", err
	}
	prompt := PromptHeader + normalizeTranscript(events)
	out, err := g.summarizer.Summarize(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("summarizer falhou: %w", err)
	}
	if err := g.store.SetSessionSummary(sessionID, out); err != nil {
		return "", err
	}
	return out, nil
}

func normalizeTranscript(events []*store.TranscriptEvent) string {
	if len(events) == 0 {
		return "(sessão sem eventos de transcript)\n"
	}
	var sb strings.Builder
	for _, ev := range events {
		sb.WriteString(fmt.Sprintf("[%s/%s] %s\n", ev.Role, ev.Kind, ev.Content))
	}
	return sb.String()
}
