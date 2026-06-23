// internal/streamengine/opencode_acp_test.go
package streamengine

import (
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/agui"
)

func newTestACP() *acpSession {
	return &acpSession{id: "s1", state: agui.StateWorking, chunks: map[string]string{}}
}

func TestACPAccumulatesMessageChunks(t *testing.T) {
	s := newTestACP()
	s.handleUpdate(map[string]any{
		"sessionUpdate": "agent_message_chunk",
		"messageId":     "m1",
		"content":       map[string]any{"type": "text", "text": "o"},
	})
	s.handleUpdate(map[string]any{
		"sessionUpdate": "agent_message_chunk",
		"messageId":     "m1",
		"content":       map[string]any{"type": "text", "text": "k"},
	})
	if got := s.Snapshot().Message; got != "ok" {
		t.Fatalf("Message = %q, quer ok", got)
	}
}

func TestACPThoughtChunkIgnored(t *testing.T) {
	s := newTestACP()
	s.handleUpdate(map[string]any{
		"sessionUpdate": "agent_thought_chunk",
		"messageId":     "m1",
		"content":       map[string]any{"type": "text", "text": "thinking"},
	})
	if got := s.Snapshot().Message; got != "" {
		t.Fatalf("thought não deveria virar Message, veio %q", got)
	}
}

func TestACPEndTurnSetsAwaiting(t *testing.T) {
	s := newTestACP()
	s.onPromptResult("end_turn")
	if got := s.Snapshot().State; got != agui.StateAwaiting {
		t.Fatalf("State = %q, quer awaiting", got)
	}
}

func TestOpencodeDriverSatisfiesDriver(t *testing.T) {
	var _ Driver = opencodeDriver{}
	var _ LiveSession = (*acpSession)(nil)
}

func TestACPCallUnblocksOnDisconnect(t *testing.T) {
	s := newTestACP()
	s.pending = map[int]chan map[string]any{}
	// registra um waiter como o call() faria
	ch := make(chan map[string]any, 1)
	s.pending[1] = ch
	// simula o drain que o readLoop faz no EOF
	s.drainPending()
	if _, ok := <-ch; ok {
		t.Fatal("canal deveria estar fechado após drainPending")
	}
}
