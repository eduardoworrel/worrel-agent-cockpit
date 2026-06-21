package memory

import (
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func ev(seq int64, role, kind, content, payload string) *store.TranscriptEvent {
	return &store.TranscriptEvent{Seq: seq, Role: role, Kind: kind, Content: content, Payload: payload}
}

func TestDetectErrorThenSuccess(t *testing.T) {
	events := []*store.TranscriptEvent{
		ev(1, "assistant", "tool_use", "Bash make build", `[{"type":"tool_use","name":"Bash","input":{"command":"make build"}}]`),
		ev(2, "user", "tool_result", "make: command not found", `[{"type":"tool_result","output":"make: command not found","is_error":true}]`),
		ev(3, "assistant", "tool_use", "Bash go build ./...", `[{"type":"tool_use","name":"Bash","input":{"command":"go build ./..."}}]`),
		ev(4, "user", "tool_result", "ok", `[{"type":"tool_result","output":"ok","is_error":false}]`),
	}
	ws := DetectFriction(events)
	if len(ws) != 1 {
		t.Fatalf("esperava 1 janela, got %d", len(ws))
	}
	if ws[0].Signal != "error_then_success" {
		t.Fatalf("signal=%q", ws[0].Signal)
	}
	// a janela deve conter o evento do erro e a tentativa seguinte
	if len(ws[0].Events) < 2 {
		t.Fatalf("janela curta: %d eventos", len(ws[0].Events))
	}
}

func TestNoWindowForIsolatedError(t *testing.T) {
	events := []*store.TranscriptEvent{
		ev(1, "assistant", "tool_use", "Bash flaky", `[{"type":"tool_use","name":"Bash","input":{"command":"flaky"}}]`),
		ev(2, "user", "tool_result", "timeout", `[{"type":"tool_result","output":"timeout","is_error":true}]`),
	}
	if ws := DetectFriction(events); len(ws) != 0 {
		t.Fatalf("erro isolado (sem resolução) não deveria gerar janela, got %d", len(ws))
	}
}
