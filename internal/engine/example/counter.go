// Package example traz o motor no-op example-counter, que prova a fiação do
// framework (registry → config → execução → sugestão) sem nenhuma chamada de
// LLM. Será removido/substituído pelo Motor de Memória (SP3).
package example

import (
	"context"
	"fmt"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/engine"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

type Counter struct{}

func (Counter) Spec() engine.Spec {
	return engine.Spec{
		ID:          "example-counter",
		Name:        "Contador de exemplo",
		Description: "Motor no-op de prova: conta eventos e tool calls de uma sessão e emite uma sugestão trivial. Removido no SP3.",
		Triggers:    []engine.Trigger{engine.TriggerOnDemand},
		OutputType:  "suggestion",
		DefaultOn:   false,
	}
}

func (Counter) Run(_ context.Context, rc engine.RunContext) error {
	evs, err := rc.Store.ListTranscriptEvents(rc.SessionID)
	if err != nil {
		return err
	}
	tools := 0
	for _, e := range evs {
		if e.Kind == "tool_use" {
			tools++
		}
	}
	sid := rc.SessionID
	_, err = rc.Store.CreateSuggestion(&store.Suggestion{
		ProjectID: rc.ProjectID,
		SessionID: &sid,
		Type:      "add_memory",
		Title:     fmt.Sprintf("%d eventos, %d tool calls", len(evs), tools),
		Origin:    "engine:example-counter",
	})
	return err
}
