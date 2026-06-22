// Package friction implementa o Motor de Fricção: roteia sinais de atrito para o
// destino certo (memória / nova / refinar skill / refinar agente / saúde) — SP5.
package friction

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
)

type Headless interface {
	RunHeadless(ctx context.Context, prompt string, opts adapter.HeadlessOpts) (string, error)
}

// Signal é um sinal de atrito coletado dos detectores (SP3/SP4) ou da saúde.
type Signal struct {
	Kind string `json:"kind"` // error_then_success | user_steps | health
	Text string `json:"text"`
}

type MemoryAction struct {
	Content  string `json:"content"`
	Category string `json:"category"`
}
type SkillAction struct {
	SkillID       string `json:"skill_id"`       // refine_skill: alvo
	Title         string `json:"title"`          // new: título do fluxo
	Content       string `json:"content"`        // refine_skill: novo conteúdo
	ChangeSummary string `json:"change_summary"`
	Signature     string `json:"signature"` // new: assinatura p/ acúmulo
}
type AgentAction struct {
	TargetAgentID string `json:"target_agent_id"`
	Persona       string `json:"persona"`
	ChangeSummary string `json:"change_summary"`
}
type HealthAction struct {
	SkillID string `json:"skill_id"`
	Action  string `json:"action"` // suspend
}

// Decision é o destino que o LLM escolheu para um sinal.
type Decision struct {
	Destino  string       `json:"destino"` // memory|new|refine_skill|refine_agent|health
	Memory   MemoryAction `json:"memory"`
	Skill    SkillAction  `json:"skill"`
	Agent    AgentAction  `json:"agent"`
	Health   HealthAction `json:"health"`
	Evidence string       `json:"evidence"`
}

type Router struct {
	h     Headless
	model string
}

func NewRouter(h Headless, model string) *Router { return &Router{h: h, model: model} }

func (r *Router) Route(ctx context.Context, signals []Signal, contextStr string) ([]Decision, error) {
	var b strings.Builder
	b.WriteString(`Você é o ROTEADOR de atrito. Para cada sinal, escolha o DESTINO e preencha o sub-objeto correspondente. destino ∈ {memory, new, refine_skill, refine_agent, health}. memory=fato anti-erro isolado. new=fluxo dirigido recorrente (preencha skill.title+signature). refine_skill=melhorar uma skill existente (skill.skill_id+content+change_summary). refine_agent=melhorar um agente existente (agent.target_agent_id+persona+change_summary). health=skill falhando (health.skill_id+action). Use os IDs existentes do contexto; não invente.`)
	b.WriteString("\n\n## Contexto atual (memória, skills, agents, candidatos)\n")
	b.WriteString(contextStr)
	b.WriteString("\n\n## Sinais de atrito\n")
	for _, s := range signals {
		b.WriteString("### " + s.Kind + "\n" + s.Text + "\n")
	}
	b.WriteString("\nDevolva APENAS um array JSON de objetos Decision {destino, memory{content,category}, skill{skill_id,title,content,change_summary,signature}, agent{target_agent_id,persona,change_summary}, health{skill_id,action}, evidence}. Só preencha o sub-objeto do destino escolhido.")
	raw, err := r.h.RunHeadless(ctx, b.String(), adapter.HeadlessOpts{Model: r.model})
	if err != nil {
		return nil, err
	}
	return parseDecisions(raw), nil
}

func parseDecisions(raw string) []Decision {
	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start < 0 || end <= start {
		return nil
	}
	var ds []Decision
	if json.Unmarshal([]byte(raw[start:end+1]), &ds) != nil {
		return nil
	}
	return ds
}
