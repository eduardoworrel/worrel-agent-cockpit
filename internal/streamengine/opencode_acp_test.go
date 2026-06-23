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

func TestACPToolCallRecorded(t *testing.T) {
	s := newTestACP()
	s.handleUpdate(map[string]any{
		"sessionUpdate": "tool_call",
		"toolCallId":    "tc1",
		"title":         "bash",
		"rawInput":      map[string]any{"command": "echo hi"},
		"status":        "pending",
	})
	tcs := s.Snapshot().ToolCalls
	if len(tcs) != 1 || tcs[0].Name != "bash" {
		t.Fatalf("ToolCalls = %+v, quer 1 com Name=bash", tcs)
	}
}

func TestACPPermissionRequestRaisesInterrupt(t *testing.T) {
	s := newTestACP()
	s.handlePermissionRequest(map[string]any{
		"id": float64(7),
		"params": map[string]any{
			"toolCall": map[string]any{"title": "bash"},
			"options": []any{
				map[string]any{"optionId": "allow-once", "kind": "allow_once"},
				map[string]any{"optionId": "reject-once", "kind": "reject_once"},
			},
		},
	})
	snap := s.Snapshot()
	if snap.Interrupt == nil || snap.Interrupt.Kind != agui.KindPermission {
		t.Fatalf("quer Interrupt de permissão, veio %+v", snap.Interrupt)
	}
	if snap.State != agui.StateAwaiting {
		t.Fatalf("State = %q, quer awaiting", snap.State)
	}
}

func TestACPRespondClearsInterrupt(t *testing.T) {
	s := newTestACP()
	s.pending = map[int]chan map[string]any{}
	// simula permissão pendente
	s.permID = 7
	s.permOpts = []acpPermOption{{OptionID: "allow-once", Kind: "allow_once"}, {OptionID: "reject-once", Kind: "reject_once"}}
	s.interrupt = &agui.Interrupt{Kind: agui.KindPermission}
	// captura o que seria enviado: substituímos enc por um buffer via test hook
	sent := captureACPWrite(s)
	if err := s.Respond(true); err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Interrupt != nil {
		t.Fatal("Interrupt deveria ser limpo após Respond")
	}
	if got := sent.lastOptionID(); got != "allow-once" {
		t.Fatalf("optionId enviado = %q, quer allow-once", got)
	}
}

func TestACPEndTurnPersistsMessage(t *testing.T) {
	s := newTestACP()
	var persisted []string
	s.persist = func(role, text string) { persisted = append(persisted, role+":"+text) }
	s.handleUpdate(map[string]any{"sessionUpdate": "agent_message_chunk", "messageId": "m1",
		"content": map[string]any{"type": "text", "text": "done"}})
	s.onPromptResult("end_turn")
	if len(persisted) != 1 || persisted[0] != "ai:done" {
		t.Fatalf("persisted = %v, quer [ai:done]", persisted)
	}
}

type acpWriteCapture struct{ msgs []map[string]any }

func (c *acpWriteCapture) lastOptionID() string {
	for i := len(c.msgs) - 1; i >= 0; i-- {
		if r, ok := c.msgs[i]["result"].(map[string]any); ok {
			if o, ok := r["outcome"].(map[string]any); ok {
				if id, ok := o["optionId"].(string); ok {
					return id
				}
			}
		}
	}
	return ""
}

// captureACPWrite injeta um writer de teste no acpSession via o campo writeFn.
func captureACPWrite(s *acpSession) *acpWriteCapture {
	c := &acpWriteCapture{}
	s.writeFn = func(v any) error {
		if m, ok := v.(map[string]any); ok {
			c.msgs = append(c.msgs, m)
		}
		return nil
	}
	return c
}

func TestACPRespondNoPendingErrors(t *testing.T) {
	s := newTestACP()
	s.pending = map[int]chan map[string]any{}
	before := s.Snapshot().State
	if err := s.Respond(true); err == nil {
		t.Fatal("Respond sem permissão pendente deveria retornar erro")
	}
	if s.Snapshot().State != before {
		t.Fatalf("State não deveria mudar quando não há permissão pendente: %q -> %q", before, s.Snapshot().State)
	}
}
