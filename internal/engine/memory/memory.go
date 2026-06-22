package memory

import (
	"context"
	"encoding/json"
	"fmt"

	eng "github.com/eduardoworrel/worrel-agent-cockpit/internal/engine"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

const defaultPrompt = `Você analisa trechos de ATRITO de uma sessão de agente e extrai "golden truths" ANTI-ERRO: verdades que, se o agente soubesse antes, teriam evitado o erro ou a re-derivação. Devolva APENAS um array JSON de objetos {content, category, evidence, related_entry_id}. category ∈ {convencao, arquitetura, gotcha, never_do, decisao}. content é enxuto e acionável. related_entry_id só quando refina/contradiz uma entrada da memória atual (senão ""). Não invente; não repita o que já está na memória atual.`

// Engine é o Motor de Memória.
type Engine struct {
	h Headless
}

func New(h Headless) *Engine { return &Engine{h: h} }

func (e *Engine) Spec() eng.Spec {
	return eng.Spec{
		ID:          "memory",
		Name:        "Motor de Memória",
		Description: "Destila golden truths anti-erro do transcript (padrão erro→correção) como entradas de memória.",
		Triggers:    []eng.Trigger{eng.TriggerProjectOpenClose, eng.TriggerOnDemand},
		Prompts:     []eng.ConfigField{{Key: "prompt", Label: "Prompt do destilador", Type: "textarea", Default: defaultPrompt}},
		Config: []eng.ConfigField{
			{Key: "detection_mode", Label: "Modo de detecção", Type: "select", Default: "hybrid", Options: []string{"hybrid", "llm_full", "heuristic_only"}},
			{Key: "delivery", Label: "Entrega", Type: "select", Default: "always_inject", Options: []string{"always_inject", "on_demand"}},
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
		truths, err = NewLLMDistiller(e.h, prompt).Distill(ctx, win, current)
		if err != nil {
			return err
		}
	default: // hybrid
		windows := DetectFriction(events)
		if len(windows) == 0 {
			return nil
		}
		truths, err = NewLLMDistiller(e.h, prompt).Distill(ctx, windows, current)
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
