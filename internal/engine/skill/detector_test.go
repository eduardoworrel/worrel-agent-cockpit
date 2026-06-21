package skill

import (
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func ev(seq int64, role, kind, content, payload string) *store.TranscriptEvent {
	return &store.TranscriptEvent{Seq: seq, Role: role, Kind: kind, Content: content, Payload: payload}
}

func TestDetectUserDirectedSteps(t *testing.T) {
	events := []*store.TranscriptEvent{
		ev(1, "user", "text", "primeiro roda o lint, depois o build, então faz o deploy", ""),
		ev(2, "assistant", "tool_use", "Bash lint", `[{"type":"tool_use","name":"Bash"}]`),
		ev(3, "assistant", "tool_use", "Bash build", `[{"type":"tool_use","name":"Bash"}]`),
		ev(4, "assistant", "tool_use", "Bash deploy", `[{"type":"tool_use","name":"Bash"}]`),
	}
	ws := DetectWorkflows(events)
	if len(ws) != 1 {
		t.Fatalf("esperava 1 janela, got %d", len(ws))
	}
	if ws[0].Signal != "user_steps" || len(ws[0].Events) < 2 {
		t.Fatalf("window=%+v", ws[0])
	}
}

func TestNoWindowForBareDelegation(t *testing.T) {
	events := []*store.TranscriptEvent{
		ev(1, "user", "text", "resolve o bug do login", ""),
		ev(2, "assistant", "tool_use", "Bash test", `[{"type":"tool_use","name":"Bash"}]`),
	}
	if ws := DetectWorkflows(events); len(ws) != 0 {
		t.Fatalf("delegação simples não deveria gerar janela, got %d", len(ws))
	}
}
