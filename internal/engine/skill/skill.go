package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strconv"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	eng "github.com/eduardoworrel/worrel-agent-cockpit/internal/engine"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// hashStr devolve um hash fnv32a hex do conteúdo — usado p/ assinatura heurística
// estável por CONTEÚDO (não por tamanho, que colidiria fluxos distintos).
func hashStr(s string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum32())
}

const defaultSkillPrompt = `Redija como WORKFLOW: passos numerados que o usuário dirigiu, inputs declarados, edge cases, critério de conclusão. content = markdown legível; structured = JSON {inputs,steps,edge_cases,completion,own_memory}. own_memory = gotchas/notas do fluxo.`
const defaultAgentPrompt = `Redija como PERSONA/PAPEL: quem o agente é, expertise, postura e regras de comportamento — NÃO uma tarefa a executar. persona = texto do system prompt.`

type Engine struct {
	h   Headless
	reg *adapter.Registry
}

func New(h Headless) *Engine { return &Engine{h: h} }

// WithRegistry habilita a escolha de harness (adapter) via config["harness"].
func (e *Engine) WithRegistry(r *adapter.Registry) *Engine { e.reg = r; return e }

func (e *Engine) llm(cfg map[string]string) (Headless, string) {
	h := e.h
	if e.reg != nil && cfg["harness"] != "" {
		if ad, ok := e.reg.Get(cfg["harness"]); ok && ad.Capabilities().Headless {
			h = ad
		}
	}
	return h, cfg["model"]
}

func (e *Engine) Spec() eng.Spec {
	return eng.Spec{
		ID:          "skill",
		Name:        "Motor de Skill/Agente",
		Description: "Detecta workflows dirigidos pelo usuário, acumula recorrência entre sessões e os matura em skills ou agentes.",
		Triggers:    []eng.Trigger{eng.TriggerProjectOpenClose, eng.TriggerRealtime, eng.TriggerOnDemand},
		Prompts: []eng.ConfigField{
			{Key: "skill_prompt", Label: "Prompt do rascunho de Skill", Type: "textarea", Default: defaultSkillPrompt},
			{Key: "agent_prompt", Label: "Prompt do rascunho de Agente", Type: "textarea", Default: defaultAgentPrompt},
		},
		Config: append([]eng.ConfigField{
			{Key: "detection_mode", Label: "Modo de detecção", Type: "select", Default: "hybrid", Options: eng.DetectionModeOptions},
			{Key: "maturation_threshold", Label: "Sessões p/ maturar", Type: "number", Default: "2"},
		}, eng.LLMFields()...),
		OutputType: "suggestion",
		DefaultOn:  false,
	}
}

func (e *Engine) Run(ctx context.Context, rc eng.RunContext) error {
	// skill_candidates.project_id é NOT NULL REFERENCES projects(id): uma sessão
	// sem projeto não pode gerar candidato project-scoped. Ignora sem erro (e sem
	// gastar LLM) — caso contrário o INSERT viola o FK (787) e o scheduler re-tenta
	// a cada tick, repetindo a falha.
	if rc.ProjectID == "" {
		return nil
	}
	events, err := rc.Store.ListTranscriptEvents(rc.SessionID)
	if err != nil {
		return err
	}
	mode := rc.Config["detection_mode"]
	threshold, _ := strconv.Atoi(rc.Config["maturation_threshold"])
	if threshold <= 0 {
		threshold = 2
	}
	skillPrompt := rc.Config["skill_prompt"]
	if skillPrompt == "" {
		skillPrompt = defaultSkillPrompt
	}
	agentPrompt := rc.Config["agent_prompt"]
	if agentPrompt == "" {
		agentPrompt = defaultAgentPrompt
	}

	windows := DetectWorkflows(events)
	if len(windows) == 0 && mode != "llm_full" {
		return nil
	}

	candidates, err := rc.Store.ListSkillCandidates(rc.ProjectID, "accumulating")
	if err != nil {
		return err
	}

	var drafts []Draft
	switch mode {
	case "heuristic_only":
		for _, w := range windows {
			drafts = append(drafts, heuristicDraft(w))
		}
	case "llm_full":
		win := []WorkflowWindow{{Signal: "full_transcript", Events: events}}
		hl, model := e.llm(rc.Config)
		drafts, err = NewDistiller(hl, model).Distill(ctx, win, candidates, skillPrompt, agentPrompt)
		if err != nil {
			return err
		}
	default: // hybrid
		hl, model := e.llm(rc.Config)
		drafts, err = NewDistiller(hl, model).Distill(ctx, windows, candidates, skillPrompt, agentPrompt)
		if err != nil {
			return err
		}
	}

	for _, dr := range drafts {
		if dr.Signature == "" {
			continue
		}
		draftJSON, _ := json.Marshal(dr)
		c, err := rc.Store.UpsertSkillCandidate(rc.ProjectID, dr.Signature, dr.Title, string(draftJSON),
			store.CandidateOccurrence{SessionID: rc.SessionID, Signal: "user_steps"})
		if err != nil {
			return err
		}
		// maturação: só matura uma vez (status accumulating → matured)
		if c.Status == "accumulating" && (c.Occurrences >= int64(threshold) || c.ExplicitMark == 1) {
			if err := rc.Store.MatureSkillCandidate(c.ID); err != nil {
				return err
			}
			if err := e.emit(rc, c, dr); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *Engine) emit(rc eng.RunContext, c *store.SkillCandidate, dr Draft) error {
	payload, _ := json.Marshal(map[string]any{
		"title": dr.Title, "signature": dr.Signature,
		"skill_draft": dr.SkillDraft, "agente_draft": dr.AgenteDraft,
		"evidence": json.RawMessage(c.Evidence),
	})
	title := dr.Title
	if r := []rune(title); len(r) > 80 {
		title = string(r[:80])
	}
	sid := rc.SessionID
	_, err := rc.Store.CreateSuggestion(&store.Suggestion{
		ProjectID: rc.ProjectID, SessionID: &sid,
		Type: "skill_or_agente_candidate", Title: title, Payload: string(payload), Origin: "engine:skill",
	})
	return err
}

// heuristicDraft monta um rascunho cru sem LLM (modo degradado).
func heuristicDraft(w WorkflowWindow) Draft {
	var cmds []string
	for _, ev := range w.Events {
		if ev.Kind == "tool_use" {
			cmds = append(cmds, ev.Content)
		}
	}
	title := "Fluxo: " + fmt.Sprintf("%v", cmds)
	sig := "heur-" + hashStr(title)
	content := "## Passos\n"
	for _, c := range cmds {
		content += "- " + c + "\n"
	}
	return Draft{
		Signature:   sig,
		Title:       title,
		SkillDraft:  SkillDraft{Name: title, Content: content, Structured: "{}"},
		AgenteDraft: AgenteDraft{Name: title, Persona: "Persona derivada do fluxo: " + content},
	}
}
