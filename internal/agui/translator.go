package agui

import (
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/ask"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// maxToolCalls limita quantos tool_use do último turno do agente entram no
// Snapshot — o suficiente para dar contexto ("o que a IA fez") sem poluir.
const maxToolCalls = 5

// Build traduz os sinais existentes (transcript, asks pendentes, encerramento)
// no contrato AG-UI que a Home consome. É puro/determinístico: toda a I/O fica
// na borda HTTP. `pending` deve conter apenas asks da sessão (ou será filtrado).
func Build(sessionID string, ended bool, events []*store.TranscriptEvent, pending []ask.Request) Snapshot {
	snap := Snapshot{SessionID: sessionID, State: stateOf(sessionID, ended, events)}
	snap.Message = lastText(events, "assistant")
	snap.UserMessage = lastText(events, "user")
	snap.ToolCalls = trailingToolCalls(events)
	snap.Interrupt = firstInterrupt(sessionID, pending)
	snap.History = historyOf(events)
	return snap
}

// historyOf reconstrói o transcript de conversa (visão de chat) a partir dos
// eventos kind="history" persistidos pelo motor stream-json. É o que faz o chat
// reaparecer após o restart do app, quando a sessão não está mais viva na
// memória do motor. Sessões legadas (sem esses eventos) ficam com History vazio.
func historyOf(events []*store.TranscriptEvent) []HistoryLine {
	var out []HistoryLine
	for _, e := range events {
		if e.Kind == "history" {
			out = append(out, HistoryLine{Role: e.Role, Text: e.Content})
		}
	}
	return out
}

// stateOf deriva o estado macro: encerrada > vez do usuário (último evento é do
// assistant) > trabalhando. Espelha a semântica de adapter.AwaitingInput.
func stateOf(_ string, ended bool, events []*store.TranscriptEvent) State {
	if ended {
		return StateEnded
	}
	for i := len(events) - 1; i >= 0; i-- {
		switch events[i].Role {
		case "assistant":
			return StateAwaiting
		case "user":
			return StateWorking
		}
	}
	return StateWorking
}

// lastText devolve o Content do último evento `text` do papel pedido. Para
// "user" ignora tool_result (que também tem Role=user) — queremos o pedido real.
func lastText(events []*store.TranscriptEvent, role string) string {
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if e.Role != role || e.Kind != "text" {
			continue
		}
		if t := strings.TrimSpace(e.Content); t != "" {
			return t
		}
	}
	return ""
}

// trailingToolCalls coleta os tool_use do assistant no fim do transcript (o
// último turno), em ordem cronológica. Para ao encontrar um texto do usuário.
func trailingToolCalls(events []*store.TranscriptEvent) []ToolCall {
	var out []ToolCall
	for i := len(events) - 1; i >= 0 && len(out) < maxToolCalls; i-- {
		e := events[i]
		if e.Role == "user" && e.Kind == "text" {
			break // começo do turno atual do agente
		}
		if e.Role == "assistant" && e.Kind == "tool_use" {
			out = append(out, toolCallOf(e.Content))
		}
	}
	// out está em ordem reversa (do fim p/ o começo) → inverte p/ cronológica.
	for l, r := 0, len(out)-1; l < r; l, r = l+1, r-1 {
		out[l], out[r] = out[r], out[l]
	}
	return out
}

// toolCallOf separa "Name <input json>" (formato gravado pelo adapter) em
// nome + resumo curto.
func toolCallOf(content string) ToolCall {
	content = strings.TrimSpace(content)
	name, summary, _ := strings.Cut(content, " ")
	summary = strings.TrimSpace(summary)
	if len(summary) > 120 {
		summary = summary[:119] + "…"
	}
	return ToolCall{Name: name, Summary: summary}
}

// firstInterrupt mapeia o primeiro ask pendente da sessão para um Interrupt.
func firstInterrupt(sessionID string, pending []ask.Request) *Interrupt {
	for _, r := range pending {
		if r.SessionID != sessionID {
			continue
		}
		return &Interrupt{
			RequestID: r.ID,
			Kind:      interruptKind(r),
			Prompt:    r.Title,
			Detail:    r.Detail,
			Options:   r.Options,
		}
	}
	return nil
}

// interruptKind classifica como o usuário responde: permission (allow/deny),
// choice (uma opção) ou text (livre).
func interruptKind(r ask.Request) InterruptKind {
	if r.Kind == "permission" {
		return KindPermission
	}
	if len(r.Options) > 0 {
		return KindChoice
	}
	return KindText
}
