package memory

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	eng "github.com/eduardoworrel/worrel-agent-cockpit/internal/engine"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

const defaultPrompt = `Você analisa trechos de ATRITO de uma sessão de agente e extrai "golden truths" ANTI-ERRO: verdades que, se o agente soubesse antes, teriam evitado o erro ou a re-derivação. Devolva APENAS um array JSON de objetos {content, category, evidence, related_entry_id}. category ∈ {convencao, arquitetura, gotcha, never_do, decisao}. content é enxuto e acionável. related_entry_id só quando refina/contradiz uma entrada da memória atual (senão ""). Não invente; não repita o que já está na memória atual.`

// Engine é o Motor de Memória.
type Engine struct {
	h   Headless
	reg *adapter.Registry // opcional: permite escolher o harness por config
}

func New(h Headless) *Engine { return &Engine{h: h} }

// WithRegistry habilita a escolha de harness (adapter) via config["harness"].
func (e *Engine) WithRegistry(r *adapter.Registry) *Engine { e.reg = r; return e }

// llm resolve o executor (harness) e o modelo a partir da config.
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
		ID:          "memory",
		Name:        "Motor de Memória",
		Description: "Destila golden truths anti-erro do transcript (padrão erro→correção) como entradas de memória.",
		Triggers:    []eng.Trigger{eng.TriggerProjectOpenClose, eng.TriggerRealtime, eng.TriggerAgentSelf, eng.TriggerOnDemand},
		Prompts:     []eng.ConfigField{{Key: "prompt", Label: "Prompt do destilador", Type: "textarea", Default: defaultPrompt}},
		Config: append([]eng.ConfigField{
			{Key: "detection_mode", Label: "Modo de detecção", Type: "select", Default: "hybrid", Options: eng.DetectionModeOptions},
			{Key: "delivery", Label: "Entrega", Type: "select", Default: "always_inject", Options: []eng.ConfigOption{
				{Value: "always_inject", Label: "Sempre injetar", Description: "A memória é injetada automaticamente no início de cada sessão (vira o primer)."},
				{Value: "on_demand", Label: "Sob demanda", Description: "Não injeta; o agente busca a memória via MCP (get_memory) quando precisar."},
			}},
		}, eng.LLMFields()...),
		OutputType: "suggestion",
		DefaultOn:  false,
	}
}

func (e *Engine) Run(ctx context.Context, rc eng.RunContext) error {
	events, err := rc.Store.ListTranscriptEvents(rc.SessionID)
	if err != nil {
		return err
	}
	current, err := rc.Store.ListMemoryEntries(rc.ProjectID, false)
	if err != nil {
		return err
	}
	mode := rc.Config["detection_mode"]
	prompt := rc.Config["prompt"]
	if prompt == "" {
		prompt = defaultPrompt
	}

	var truths []GoldenTruth
	switch mode {
	case "heuristic_only":
		for _, w := range DetectFriction(events) {
			truths = append(truths, heuristicTruth(w))
		}
	case "llm_full":
		// uma "janela" única com todos os eventos
		win := []FrictionWindow{{Signal: "full_transcript", Events: events}}
		hl, model := e.llm(rc.Config)
		truths, err = NewLLMDistiller(hl, prompt, model).Distill(ctx, win, current)
		if err != nil {
			return err
		}
	default: // hybrid
		windows := DetectFriction(events)
		if len(windows) == 0 {
			return nil
		}
		hl, model := e.llm(rc.Config)
		truths, err = NewLLMDistiller(hl, prompt, model).Distill(ctx, windows, current)
		if err != nil {
			return err
		}
	}

	validIDs := map[string]bool{}
	for _, c := range current {
		validIDs[c.ID] = true
	}
	sid := rc.SessionID
	for _, gt := range truths {
		if gt.Content == "" {
			continue
		}
		if gt.RelatedEntryID != "" && !validIDs[gt.RelatedEntryID] {
			gt.RelatedEntryID = "" // LLM apontou id inexistente → sem conflito
		}
		payload, _ := json.Marshal(gt)
		title := gt.Content
		if r := []rune(title); len(r) > 80 {
			title = string(r[:80])
		}
		if _, err := rc.Store.CreateSuggestion(&store.Suggestion{
			ProjectID: rc.ProjectID,
			SessionID: &sid,
			Type:      "add_memory_entry",
			Title:     title,
			Payload:   string(payload),
			Origin:    "engine:memory",
		}); err != nil {
			return err
		}
	}
	return nil
}

// heuristicTruth monta um golden truth cru a partir de uma janela (modo sem LLM).
func heuristicTruth(w FrictionWindow) GoldenTruth {
	var failed, fixed string
	for i, ev := range w.Events {
		if ok, _ := eventToolUse(ev); ok {
			if failed == "" {
				failed = ev.Content
			} else {
				fixed = ev.Content
			}
		}
		_ = i
	}
	content := fmt.Sprintf("Comando que falhou: %q; tentativa seguinte: %q.", failed, fixed)
	return GoldenTruth{Content: content, Category: "gotcha", Evidence: w.Signal}
}
