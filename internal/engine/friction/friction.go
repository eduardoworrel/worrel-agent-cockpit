package friction

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strconv"

	eng "github.com/eduardoworrel/worrel-agent-cockpit/internal/engine"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/engine/memory"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/engine/skill"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// hashStr: hash fnv32a hex do conteúdo (assinatura heurística estável por conteúdo).
func hashStr(s string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum32())
}

const defaultPrompt = `Roteie cada sinal de atrito para o destino certo (memory/new/refine_skill/refine_agent/health), preenchendo só o sub-objeto do destino. Use IDs existentes do contexto.`

type Engine struct{ h Headless }

func New(h Headless) *Engine { return &Engine{h: h} }

func (e *Engine) Spec() eng.Spec {
	return eng.Spec{
		ID:          "friction",
		Name:        "Motor de Fricção",
		Description: "Roteia sinais de atrito para memória / nova skill / refinar skill ou agente / saúde de skill.",
		Triggers:    []eng.Trigger{eng.TriggerProjectOpenClose, eng.TriggerOnDemand},
		Prompts:     []eng.ConfigField{{Key: "prompt", Label: "Prompt do roteador", Type: "textarea", Default: defaultPrompt}},
		Config: []eng.ConfigField{
			{Key: "detection_mode", Label: "Modo de detecção", Type: "select", Default: "hybrid", Options: eng.DetectionModeOptions},
			{Key: "health_consec_failures", Label: "Falhas consecutivas p/ saúde", Type: "number", Default: "2"},
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
	// 1) coleta sinais reusando os detectores SP3/SP4
	var signals []Signal
	for _, w := range memory.DetectFriction(events) {
		signals = append(signals, Signal{Kind: w.Signal, Text: windowText(w.Events)})
	}
	for _, w := range skill.DetectWorkflows(events) {
		signals = append(signals, Signal{Kind: w.Signal, Text: windowText(w.Events)})
	}
	// 2) passe de saúde: skills com falhas consecutivas >= limiar
	thr, _ := strconv.Atoi(rc.Config["health_consec_failures"])
	if thr <= 0 {
		thr = 2
	}
	skills, _ := rc.Store.ListSkills(rc.ProjectID)
	healthOf := map[string]string{} // signalText -> skillID (para o emit de health)
	for _, sk := range skills {
		if cf, _ := rc.Store.ConsecutiveFailures(sk.ID); cf >= thr {
			txt := fmt.Sprintf("skill %q (id %s) com %d falhas consecutivas", sk.Name, sk.ID, cf)
			signals = append(signals, Signal{Kind: "health", Text: txt})
			healthOf[txt] = sk.ID
		}
	}
	if len(signals) == 0 {
		return nil
	}

	mode := rc.Config["detection_mode"]
	var decisions []Decision
	if mode == "heuristic_only" {
		decisions = heuristicRoute(signals, healthOf)
	} else {
		decisions, err = NewRouter(e.h).Route(ctx, signals, buildContext(rc))
		if err != nil {
			return err
		}
	}

	// validação de alvos contra o store
	validSkills := map[string]bool{}
	for _, sk := range skills {
		validSkills[sk.ID] = true
	}
	agents, _ := rc.Store.ListAgents(rc.ProjectID)
	validAgents := map[string]bool{}
	for _, a := range agents {
		validAgents[a.ID] = true
	}

	sid := rc.SessionID
	for _, d := range decisions {
		if err := e.emit(rc, &sid, d, validSkills, validAgents); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) emit(rc eng.RunContext, sid *string, d Decision, validSkills, validAgents map[string]bool) error {
	mk := func(typ, title, payload string, skillID *string) error {
		_, err := rc.Store.CreateSuggestion(&store.Suggestion{
			ProjectID: rc.ProjectID, SessionID: sid, SkillID: skillID,
			Type: typ, Title: clip(title), Payload: payload, Origin: "engine:friction",
		})
		return err
	}
	switch d.Destino {
	case "memory":
		if d.Memory.Content == "" {
			return nil
		}
		pl, _ := json.Marshal(map[string]string{"content": d.Memory.Content, "category": d.Memory.Category, "evidence": d.Evidence})
		return mk("add_memory_entry", d.Memory.Content, string(pl), nil)
	case "new":
		if d.Skill.Signature == "" {
			return nil
		}
		draft, _ := json.Marshal(map[string]any{"title": d.Skill.Title, "signature": d.Skill.Signature})
		_, err := rc.Store.UpsertSkillCandidate(rc.ProjectID, d.Skill.Signature, d.Skill.Title, string(draft),
			store.CandidateOccurrence{SessionID: *sid, Signal: "friction"})
		return err
	case "refine_skill":
		if !validSkills[d.Skill.SkillID] {
			return nil
		}
		pl, _ := json.Marshal(map[string]string{"content": d.Skill.Content, "change_summary": d.Skill.ChangeSummary, "evidence": d.Evidence})
		skid := d.Skill.SkillID
		return mk("skill.correction", "refinar skill", string(pl), &skid)
	case "refine_agent":
		if !validAgents[d.Agent.TargetAgentID] {
			return nil
		}
		pl, _ := json.Marshal(map[string]string{"target_agent_id": d.Agent.TargetAgentID, "persona": d.Agent.Persona, "change_summary": d.Agent.ChangeSummary, "evidence": d.Evidence})
		return mk("agent.correction", "refinar agente", string(pl), nil)
	case "health":
		if !validSkills[d.Health.SkillID] {
			return nil
		}
		pl, _ := json.Marshal(map[string]string{"skill_id": d.Health.SkillID, "action": "suspend", "evidence": d.Evidence})
		skid := d.Health.SkillID
		return mk("skill.health", "saúde de skill", string(pl), &skid)
	}
	return nil
}

func windowText(events []*store.TranscriptEvent) string {
	s := ""
	for _, ev := range events {
		s += ev.Role + "/" + ev.Kind + ": " + ev.Content + "\n"
	}
	return s
}

func buildContext(rc eng.RunContext) string {
	s := "Skills:\n"
	if sks, _ := rc.Store.ListSkills(rc.ProjectID); sks != nil {
		for _, sk := range sks {
			s += "- [" + sk.ID + "] " + sk.Name + "\n"
		}
	}
	s += "Agents:\n"
	if ags, _ := rc.Store.ListAgents(rc.ProjectID); ags != nil {
		for _, a := range ags {
			s += "- [" + a.ID + "] " + a.Name + "\n"
		}
	}
	s += "Memória:\n"
	if ents, _ := rc.Store.ListMemoryEntries(rc.ProjectID, false); ents != nil {
		for _, en := range ents {
			s += "- (" + en.Category + ") " + en.Content + "\n"
		}
	}
	return s
}

// heuristicRoute roteia sem LLM (modo degradado): error_then_success→memory;
// user_steps→new; health→health.
func heuristicRoute(signals []Signal, healthOf map[string]string) []Decision {
	var out []Decision
	for _, s := range signals {
		switch s.Kind {
		case "error_then_success":
			out = append(out, Decision{Destino: "memory", Memory: MemoryAction{Content: s.Text, Category: "gotcha"}, Evidence: s.Kind})
		case "user_steps":
			out = append(out, Decision{Destino: "new", Skill: SkillAction{Title: clip(s.Text), Signature: "heur-" + hashStr(s.Text)}, Evidence: s.Kind})
		case "health":
			out = append(out, Decision{Destino: "health", Health: HealthAction{SkillID: healthOf[s.Text], Action: "suspend"}, Evidence: s.Kind})
		}
	}
	return out
}

func clip(s string) string {
	if r := []rune(s); len(r) > 80 {
		return string(r[:80])
	}
	return s
}
