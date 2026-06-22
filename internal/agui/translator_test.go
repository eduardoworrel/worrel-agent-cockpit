package agui

import (
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/ask"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func ev(role, kind, content string) *store.TranscriptEvent {
	return &store.TranscriptEvent{Role: role, Kind: kind, Content: content}
}

func TestBuild_HistoryFromPersistedEvents(t *testing.T) {
	// Eventos kind="history" persistidos pelo motor stream-json (sobreviver ao
	// restart): Build reconstrói o transcript de chat na ordem, ignorando outros.
	events := []*store.TranscriptEvent{
		ev("you", "history", "rode os testes"),
		ev("ai", "history", "claro, rodando"),
		ev("tool", "history", "Bash {\"command\":\"go test\"}"),
		ev("assistant", "text", "ignore: não é history"),
		ev("system", "history", "você permitiu Bash"),
	}
	snap := Build("s1", true, events, nil)

	want := []HistoryLine{
		{Role: "you", Text: "rode os testes"},
		{Role: "ai", Text: "claro, rodando"},
		{Role: "tool", Text: "Bash {\"command\":\"go test\"}"},
		{Role: "system", Text: "você permitiu Bash"},
	}
	if len(snap.History) != len(want) {
		t.Fatalf("History len = %d, want %d (%+v)", len(snap.History), len(want), snap.History)
	}
	for i, w := range want {
		if snap.History[i] != w {
			t.Errorf("History[%d] = %+v, want %+v", i, snap.History[i], w)
		}
	}
}

func TestBuild_AwaitingWithContext(t *testing.T) {
	events := []*store.TranscriptEvent{
		ev("user", "text", "faça o update no db"),
		ev("assistant", "tool_use", "Bash {\"command\":\"psql -c '...'\"}"),
		ev("assistant", "tool_use", "Read {\"file_path\":\"a.go\"}"),
		ev("assistant", "text", "encontrei a senha e fiz o update. confirma?"),
	}
	snap := Build("s1", false, events, nil)

	if snap.State != StateAwaiting {
		t.Fatalf("state = %q, want awaiting (último evento é assistant)", snap.State)
	}
	if snap.Message != "encontrei a senha e fiz o update. confirma?" {
		t.Fatalf("message = %q", snap.Message)
	}
	if snap.UserMessage != "faça o update no db" {
		t.Fatalf("user_message = %q", snap.UserMessage)
	}
	if len(snap.ToolCalls) != 2 || snap.ToolCalls[0].Name != "Bash" || snap.ToolCalls[1].Name != "Read" {
		t.Fatalf("tool_calls = %+v (esperava Bash, Read em ordem)", snap.ToolCalls)
	}
	if snap.Interrupt != nil {
		t.Fatalf("interrupt = %+v, want nil", snap.Interrupt)
	}
	if !snap.NeedsAttention() {
		t.Fatal("awaiting deve acender o ⚠️")
	}
}

func TestBuild_WorkingIgnoresToolResult(t *testing.T) {
	events := []*store.TranscriptEvent{
		ev("user", "text", "rode os testes"),
		ev("assistant", "tool_use", "Bash {\"command\":\"go test\"}"),
		ev("user", "tool_result", "ok"),
	}
	snap := Build("s1", false, events, nil)
	if snap.State != StateWorking {
		t.Fatalf("state = %q, want working (último é tool_result/user)", snap.State)
	}
	if snap.UserMessage != "rode os testes" {
		t.Fatalf("user_message = %q, não deve pegar o tool_result", snap.UserMessage)
	}
}

func TestBuild_Interrupts(t *testing.T) {
	cases := []struct {
		name string
		req  ask.Request
		want InterruptKind
	}{
		{"permission", ask.Request{ID: "1", SessionID: "s1", Kind: "permission", Title: "Rodar comando"}, KindPermission},
		{"choice", ask.Request{ID: "2", SessionID: "s1", Kind: "choice", Title: "Qual?", Options: []string{"a", "b"}}, KindChoice},
		{"text", ask.Request{ID: "3", SessionID: "s1", Kind: "choice", Title: "Como?"}, KindText},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			snap := Build("s1", false, nil, []ask.Request{c.req})
			if snap.Interrupt == nil || snap.Interrupt.Kind != c.want {
				t.Fatalf("interrupt = %+v, want kind %q", snap.Interrupt, c.want)
			}
		})
	}
}

func TestBuild_FiltersPendingBySession(t *testing.T) {
	pending := []ask.Request{{ID: "1", SessionID: "other", Kind: "permission", Title: "x"}}
	snap := Build("s1", false, nil, pending)
	if snap.Interrupt != nil {
		t.Fatalf("interrupt = %+v, não deve pegar ask de outra sessão", snap.Interrupt)
	}
}

func TestBuild_Ended(t *testing.T) {
	snap := Build("s1", true, []*store.TranscriptEvent{ev("assistant", "text", "tchau")}, nil)
	if snap.State != StateEnded {
		t.Fatalf("state = %q, want ended", snap.State)
	}
}
