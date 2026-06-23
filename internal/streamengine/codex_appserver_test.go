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
