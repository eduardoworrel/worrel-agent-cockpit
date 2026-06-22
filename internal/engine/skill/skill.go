package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	eng "github.com/eduardoworrel/worrel-agent-cockpit/internal/engine"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

const defaultSkillPrompt = `Redija como WORKFLOW: passos numerados que o usuário dirigiu, inputs declarados, edge cases, critério de conclusão. content = markdown legível; structured = JSON {inputs,steps,edge_cases,completion,own_memory}. own_memory = gotchas/notas do fluxo.`
const defaultAgentPrompt = `Redija como PERSONA/PAPEL: quem o agente é, expertise, postura e regras de comportamento — NÃO uma tarefa a executar. persona = texto do system prompt.`

type Engine struct{ h Headless }

func New(h Headless) *Engine { return &Engine{h: h} }

func (e *Engine) Spec() eng.Spec {
	return eng.Spec{
		ID:          "skill",
		Name:        "Motor de Skill/Agente",
		Description: "Detecta workflows dirigidos pelo usuário, acumula recorrência entre sessões e os matura em skills ou agentes.",
		Triggers:    []eng.Trigger{eng.TriggerProjectOpenClose, eng.TriggerOnDemand},
		Prompts: []eng.ConfigField{
			{Key: "skill_prompt", Label: "Prompt do rascunho de Skill", Type: "textarea", Default: defaultSkillPrompt},
			{Key: "agent_prompt", Label: "Prompt do rascunho de Agente", Type: "textarea", Default: defaultAgentPrompt},
		},
		Config: []eng.ConfigField{
			{Key: "detection_mode", Label: "Modo de detecção (hybrid|llm_full|heuristic_only)", Type: "text", Default: "hybrid"},
			{Key: "maturation_threshold", Label: "Sessões p/ maturar", Type: "number", Default: "2"},
		},
		OutputType: "suggestion",
		DefaultOn:  false,
	}
}

func (e *Engine) Run(ctx context.Context, rc eng.RunContext) error {
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
		drafts, err = NewDistiller(e.h).Distill(ctx, win, candidates, skillPrompt, agentPrompt)
		if err != nil {
			return err
		}
	default: // hybrid
		drafts, err = NewDistiller(e.h).Distill(ctx, windows, candidates, skillPrompt, agentPrompt)
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
	sig := "heur-" + fmt.Sprintf("%x", len(title))
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
