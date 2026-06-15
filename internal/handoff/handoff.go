// Package handoff gera resumos estruturados de sessão (spec §9) e encadeia
// sessões quando o contexto se esgota. O resumo é produzido por um Summarizer
// (na fase 3, o adaptador headless do CLI preferido) sobre o transcript
// normalizado, e persistido em sessions.summary.
package handoff

import (
	"context"
	"fmt"
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// Summarizer roda um prompt headless e devolve texto. A fase 3 conecta isto
// ao adaptador (adapter.RunHeadless). Em teste, é fakeado.
type Summarizer interface {
	Summarize(ctx context.Context, prompt string) (string, error)
}

// LiveReader lê o transcript direto do arquivo do CLI. Sessões in-app (wrapper)
// NÃO têm seu transcript ingerido em transcript_events (só o importer de
// histórico externo faz isso); logo o handoff de uma sessão viva precisa ler o
// .jsonl ao vivo. *adapter.Adapter satisfaz esta interface (tem ReadTranscript).
type LiveReader interface {
	ReadTranscript(ref adapter.SessionRef) ([]adapter.TranscriptEvent, error)
}

type Generator struct {
	store      *store.Store
	summarizer Summarizer
	live       LiveReader
}

func New(s *store.Store, sum Summarizer) *Generator {
	return &Generator{store: s, summarizer: sum}
}

// WithLiveReader liga a leitura ao vivo do transcript (sessões in-app). Opcional.
func (g *Generator) WithLiveReader(r LiveReader) *Generator {
	g.live = r
	return g
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
	// Sessão in-app não tem eventos em transcript_events: lê o .jsonl ao vivo.
	if len(events) == 0 && g.live != nil {
		if sess, err := g.store.GetSession(sessionID); err == nil {
			ref := adapter.SessionRef{Adapter: sess.Adapter, ExternalRef: externalRef(sess)}
			if live, err := g.live.ReadTranscript(ref); err == nil {
				events = fromAdapterEvents(sessionID, live)
			}
		}
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

// externalRef devolve o ref do CLI para resolver o transcript. Para sessões
// in-app é o próprio id (CreateSession já o grava); cai no id como defesa para
// linhas antigas com external_ref nulo.
func externalRef(sess *store.Session) string {
	if sess.ExternalRef != nil && *sess.ExternalRef != "" {
		return *sess.ExternalRef
	}
	return sess.ID
}

func fromAdapterEvents(sessionID string, evs []adapter.TranscriptEvent) []*store.TranscriptEvent {
	out := make([]*store.TranscriptEvent, 0, len(evs))
	for i, e := range evs {
		out = append(out, &store.TranscriptEvent{
			SessionID: sessionID, Seq: int64(i + 1),
			Role: e.Role, Kind: e.Kind, Content: e.Content,
			TokensIn: e.TokensIn, TokensOut: e.TokensOut, CreatedAt: e.CreatedAt,
		})
	}
	return out
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
