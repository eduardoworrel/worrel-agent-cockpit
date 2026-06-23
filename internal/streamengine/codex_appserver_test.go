// internal/streamengine/codex_appserver_test.go
package streamengine

import (
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/agui"
)

func newTestCodex() *codexSession {
	return &codexSession{id: "s1", state: agui.StateWorking, deltas: map[string]string{},
		pending: map[int]chan map[string]any{}}
}

func TestCodexAccumulatesAgentDeltas(t *testing.T) {
	s := newTestCodex()
	s.handleNotification("item/agentMessage/delta", map[string]any{"itemId": "i1", "delta": "o"})
	s.handleNotification("item/agentMessage/delta", map[string]any{"itemId": "i1", "delta": "k"})
	if got := s.Snapshot().Message; got != "ok" {
		t.Fatalf("Message = %q, quer ok", got)
	}
}

func TestCodexTurnCompletedSetsAwaiting(t *testing.T) {
	s := newTestCodex()
	s.handleNotification("turn/completed", map[string]any{"turn": map[string]any{"status": "completed"}})
	if got := s.Snapshot().State; got != agui.StateAwaiting {
		t.Fatalf("State = %q, quer awaiting", got)
	}
}

func TestCodexUserMessageEchoIgnored(t *testing.T) {
	s := newTestCodex()
	s.handleNotification("item/started", map[string]any{
		"item": map[string]any{"type": "userMessage",
			"content": []any{map[string]any{"type": "text", "text": "hi"}}}})
	if got := s.Snapshot().Message; got != "" {
		t.Fatalf("echo do usuário não deveria virar Message, veio %q", got)
	}
}

func TestCodexDriverSatisfiesDriver(t *testing.T) {
	var _ Driver = codexDriver{}
	var _ LiveSession = (*codexSession)(nil)
}

func TestCodexCommandItemRecorded(t *testing.T) {
	s := newTestCodex()
	s.handleNotification("item/started", map[string]any{
		"item": map[string]any{"type": "commandExecution", "id": "call_1",
			"command": "/bin/zsh -lc 'echo hi'", "status": "inProgress"}})
	tcs := s.Snapshot().ToolCalls
	if len(tcs) != 1 || tcs[0].Name != "commandExecution" {
		t.Fatalf("ToolCalls = %+v, quer 1 commandExecution", tcs)
	}
}

func TestCodexCompletedDoesNotDoubleRecord(t *testing.T) {
	s := newTestCodex()
	item := map[string]any{"item": map[string]any{"type": "commandExecution", "id": "call_1",
		"command": "echo hi", "status": "completed"}}
	s.handleNotification("item/started", item)
	s.handleNotification("item/completed", item)
	if n := len(s.Snapshot().ToolCalls); n != 1 {
		t.Fatalf("ToolCalls = %d, quer 1 (sem duplicar no completed)", n)
	}
}

func TestCodexFinishTurnIdempotent(t *testing.T) {
	s := newTestCodex()
	var persisted int
	s.persist = func(role, text string) { persisted++ }
	s.handleNotification("item/agentMessage/delta", map[string]any{"itemId": "i1", "delta": "done"})
	s.finishTurn() // primeiro fim de turno
	s.finishTurn() // segundo (ex.: turn/completed após erro) — não deve duplicar
	if persisted != 1 {
		t.Fatalf("persist chamado %d vezes, quer 1 (idempotente)", persisted)
	}
	if n := len(s.Snapshot().History); n != 1 {
		t.Fatalf("History tem %d linhas, quer 1 (sem duplicar 'ai')", n)
	}
}
