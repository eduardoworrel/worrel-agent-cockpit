// Package agui é a camada de interação AG-UI-shaped que serve EXCLUSIVAMENTE a
// Home. Ela expõe um contrato estável (Snapshot + Input) que descreve o estado
// de interação de uma sessão — o que a IA disse/fez, o último pedido do usuário
// e qualquer pergunta bloqueante pendente — sem que a Home precise conhecer o
// transcript, o PTY ou o broker de asks.
//
// Nesta v1 um Translator preenche o contrato lendo os sinais que já existem
// (transcript_events, ask.Broker, AwaitingInput). Quando o motor for redefinido
// para emitir o contrato nativamente, o Translator some e a Home não muda.
//
// O canal de terminal/PTY (/api/sessions/{id}/term) é independente e continua
// sendo a interação raw — esta camada NÃO o substitui, só cobre a Home.
package agui

import "encoding/json"

// State é o estado macro de interação da sessão.
type State string

const (
	StateWorking  State = "working"  // agente trabalhando; nada a fazer
	StateAwaiting State = "awaiting" // turno encerrado, vez do usuário (prompt livre)
	StateEnded    State = "ended"    // sessão encerrada
)

// ToolCall resume um tool_use do agente — "o que a IA fez".
type ToolCall struct {
	Name    string `json:"name"`
	Summary string `json:"summary,omitempty"`
}

// InterruptKind diz como o usuário pode responder a uma pergunta bloqueante.
type InterruptKind string

const (
	KindPermission InterruptKind = "permission" // allow/deny
	KindChoice     InterruptKind = "choice"     // uma das Options
	KindText       InterruptKind = "text"       // texto livre
)

// Interrupt é uma pergunta bloqueante pendente (do hook PreToolUse ou do MCP
// ask_user). request_id casa com o ask.Broker para responder.
type Interrupt struct {
	RequestID string        `json:"request_id"`
	Kind      InterruptKind `json:"kind"`
	Prompt    string        `json:"prompt"`
	Detail    string        `json:"detail,omitempty"`
	Options   []string      `json:"options,omitempty"`
}

// HistoryLine é uma linha do transcript da sessão (visão de conversa).
type HistoryLine struct {
	Role string `json:"role"` // you | ai | tool | system
	Text string `json:"text"`
}

// Snapshot é o estado de interação completo de UMA sessão, no formato que a Home
// consome. Espelha os tipos de evento AG-UI (MESSAGE, TOOL_CALL, STATE,
// INTERRUPT) achatados num retrato pontual (v1 sem streaming).
type Snapshot struct {
	SessionID      string          `json:"session_id"`
	State          State           `json:"state"`
	Message        string          `json:"message,omitempty"`         // última fala/pergunta do assistant
	UserMessage    string          `json:"user_message,omitempty"`    // último pedido do usuário (cru)
	RequestSummary string          `json:"request_summary,omitempty"` // pedido do usuário condensado por IA ("Seu pedido")
	ToolCalls      []ToolCall      `json:"tool_calls,omitempty"`      // tool_use recentes do assistant
	Progress       []string        `json:"progress,omitempty"`        // resumo narrado por IA (timeline do card)
	History        []HistoryLine   `json:"history,omitempty"`         // transcript completo (visão de conversa)
	Interrupt      *Interrupt      `json:"interrupt,omitempty"`       // pergunta bloqueante, ou nil
	AskHTML        string          `json:"ask_html,omitempty"`        // HTML rico do que a IA espera (render em iframe sandbox)
	ResponseWidget *ResponseWidget `json:"response_widget,omitempty"` // controle de resposta dinâmico (experimental)
}

// ResponseWidget descreve, em JSON livre (sem tipo fixo), COMO pedir o dado ao
// usuário — um experimento removível. Gerado junto com AskHTML pela engine
// ask_html. Type é o discriminador do switch no front (ex.: "range", "options");
// Spec carrega os parâmetros específicos do tipo.
type ResponseWidget struct {
	Type string          `json:"type"`
	Spec json.RawMessage `json:"spec,omitempty"`
}

// NeedsAttention indica se o ⚠️ do card deve acender.
func (s Snapshot) NeedsAttention() bool {
	return s.Interrupt != nil || s.State == StateAwaiting
}
