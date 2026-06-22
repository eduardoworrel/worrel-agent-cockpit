package skill

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// Headless é a dependência mínima de LLM (satisfeita por adapter.Adapter).
type Headless interface {
	RunHeadless(ctx context.Context, prompt string, opts adapter.HeadlessOpts) (string, error)
}

type SkillDraft struct {
	Name       string `json:"name"`
	Content    string `json:"content"`
	Structured string `json:"structured"`
}

type AgenteDraft struct {
	Name    string `json:"name"`
	Persona string `json:"persona"`
}

// Draft é um candidato redigido: assinatura semântica + um rascunho de skill E um
// de agente (o usuário escolhe o destino na revisão).
type Draft struct {
	Signature   string      `json:"signature"`
	Title       string      `json:"title"`
	SkillDraft  SkillDraft  `json:"skill_draft"`
	AgenteDraft AgenteDraft `json:"agente_draft"`
}

type Distiller struct {
	h     Headless
	model string
}

func NewDistiller(h Headless, model string) *Distiller { return &Distiller{h: h, model: model} }

func (d *Distiller) Distill(ctx context.Context, windows []WorkflowWindow, candidates []*store.SkillCandidate, skillPrompt, agentPrompt string) ([]Draft, error) {
	var b strings.Builder
	b.WriteString("Você analisa um WORKFLOW que o usuário dirigiu numa sessão e produz, para cada um, um objeto JSON com: signature (chave SEMÂNTICA estável do fluxo — REUTILIZE a signature de um candidato existente abaixo se for O MESMO fluxo, senão crie uma nova curta), title, skill_draft, agente_draft.\n")
	b.WriteString("\n## Como redigir skill_draft (workflow estruturado):\n" + skillPrompt + "\n")
	b.WriteString("\n## Como redigir agente_draft (persona/papel):\n" + agentPrompt + "\n")
	b.WriteString("\n## Candidatos existentes (reutilize a signature se casar):\n")
	for _, c := range candidates {
		b.WriteString("- [" + c.Signature + "] " + c.Title + "\n")
	}
	b.WriteString("\n## Workflows da sessão:\n")
	for _, w := range windows {
		b.WriteString("### sinal: " + w.Signal + "\n" + joinContents(w) + "\n")
	}
	b.WriteString("\nDevolva APENAS um array JSON de objetos {signature,title,skill_draft:{name,content,structured},agente_draft:{name,persona}}. structured é um JSON-string com {inputs,steps,edge_cases,completion,own_memory}.")
	raw, err := d.h.RunHeadless(ctx, b.String(), adapter.HeadlessOpts{Model: d.model})
	if err != nil {
		return nil, err
	}
	return parseDrafts(raw), nil
}

func parseDrafts(raw string) []Draft {
	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start < 0 || end <= start {
		return nil
	}
	var ds []Draft
	if json.Unmarshal([]byte(raw[start:end+1]), &ds) != nil {
		return nil
	}
	return ds
}
