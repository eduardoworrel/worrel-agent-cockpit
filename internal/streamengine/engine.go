// Package streamengine dirige uma sessão do Claude Code pelo protocolo
// stream-json (stdin/stdout), 100% nativo do CLI — sem MCP, sem hook, sem PTY.
//
// O CLI roda com:
//
//	claude -p --input-format stream-json --output-format stream-json --verbose \
//	       --permission-prompt-tool stdio --permission-mode default
//
// e emite eventos estruturados: `assistant` (texto/tool_use), `result` (fim de
// turno), e `control_request`/`can_use_tool` quando uma ferramenta precisa de
// permissão. Respondemos prompt e permissão escrevendo JSON no stdin. Tudo isso
// alimenta o contrato AG-UI (agui.Snapshot) que a Home consome.
package streamengine

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/agui"
)

// Session é um processo claude stream-json vivo, com seu estado de interação.
type Session struct {
	id    string
	cmd   *exec.Cmd
	stdin *json.Encoder
	stdinW interface{ Close() error }

	// onChange é chamado a cada mudança de estado (a Home rebusca o Snapshot).
	onChange func(sessionID string)

	mu        sync.Mutex
	message   string          // última fala do assistant
	progress  []string        // falas recentes (timeline do card)
	toolCalls []agui.ToolCall // tool_use do turno atual
	state       agui.State
	interrupt   *agui.Interrupt // can_use_tool pendente, ou nil
	reqID       any             // request_id do can_use_tool em aberto
	pendingIn   any             // input da ferramenta pendente (exigido no allow)
	pendingTool string          // nome da ferramenta pendente (p/ o log da decisão)
	history     []agui.HistoryLine // transcript completo da conversa
}

// PermissionMode controla como o CLI trata permissões. Valores válidos do CLI:
// "auto" (auto-mode: o CLI decide; pede pelo stdio só quando precisa),
// "default" (pede para toda tool), "acceptEdits", "bypassPermissions",
// "dontAsk", "plan".
type PermissionMode = string

// claudeArgs são os flags provados: stream-json bidirecional + permissão via stdio,
// no modo dado (vazio = "auto").
func claudeArgs(mode PermissionMode) []string {
	if mode == "" {
		mode = "auto"
	}
	return []string{
		"-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
		"--permission-prompt-tool", "stdio",
		"--permission-mode", mode,
		// AskUserQuestion espera uma UI interativa do CLI; no nosso fluxo ela
		// vira um "Permitir AskUserQuestion?" feio. Proibimos para o agente
		// FALAR a pergunta — que a interpretação por IA renderiza em opções.
		"--disallowedTools", "AskUserQuestion",
	}
}

// Start spawna o claude no cwd no modo de permissão dado e começa a ler o stream.
// onChange é notificado a cada transição (pode ser nil).
func Start(ctx context.Context, sessionID, cwd string, mode PermissionMode, onChange func(string)) (*Session, error) {
	cmd := exec.CommandContext(ctx, "claude", claudeArgs(mode)...)
	cmd.Dir = cwd
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	s := &Session{
		id: sessionID, cmd: cmd,
		stdin: json.NewEncoder(stdin), stdinW: stdin,
		onChange: onChange, state: agui.StateWorking,
	}
	go s.readLoop(bufio.NewReaderSize(stdout, 1<<20))
	return s, nil
}

// Snapshot devolve o estado de interação atual no contrato AG-UI.
func (s *Session) Snapshot() agui.Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return agui.Snapshot{
		SessionID: s.id,
		State:     s.state,
		Message:   s.message,
		ToolCalls: append([]agui.ToolCall(nil), s.toolCalls...),
		Progress:  append([]string(nil), s.progress...),
		History:   append([]agui.HistoryLine(nil), s.history...),
		Interrupt: s.interrupt,
	}
}

// SendPrompt manda um novo turno do usuário para a sessão.
func (s *Session) SendPrompt(text string) error {
	s.mu.Lock()
	s.state = agui.StateWorking
	s.toolCalls = nil
	s.history = append(s.history, agui.HistoryLine{Role: "you", Text: text})
	s.mu.Unlock()
	s.notify()
	return s.write(map[string]any{
		"type": "user",
		"message": map[string]any{"role": "user",
			"content": []map[string]any{{"type": "text", "text": text}}},
	})
}

// Respond responde à permissão pendente (interrupt can_use_tool): allow ou deny.
func (s *Session) Respond(allow bool) error {
	s.mu.Lock()
	reqID := s.reqID
	pendingIn := s.pendingIn
	tool := s.pendingTool
	s.interrupt = nil
	s.reqID = nil
	s.pendingIn = nil
	s.pendingTool = ""
	decision := "negou"
	if allow {
		decision = "permitiu"
		s.state = agui.StateWorking // volta a trabalhar após autorizar
	}
	s.history = append(s.history, agui.HistoryLine{Role: "system", Text: "você " + decision + " " + tool})
	s.mu.Unlock()
	if reqID == nil {
		return fmt.Errorf("nenhuma permissão pendente")
	}
	// allow EXIGE updatedInput (o input original, possivelmente ajustado); deny
	// leva uma mensagem. Espelha o protocolo do CLI (control_response).
	var inner map[string]any
	if allow {
		in := pendingIn
		if in == nil {
			in = map[string]any{}
		}
		inner = map[string]any{"behavior": "allow", "updatedInput": in}
	} else {
		inner = map[string]any{"behavior": "deny", "message": "worrel: negado pelo usuário"}
	}
	s.notify()
	// O CLI espera request_id DENTRO de response (não no topo) — senão não casa
	// o pedido e a ferramenta trava.
	return s.write(map[string]any{
		"type": "control_response",
		"response": map[string]any{
			"subtype":    "success",
			"request_id": reqID,
			"response":   inner,
		},
	})
}

// Close encerra a sessão.
func (s *Session) Close() {
	_ = s.stdinW.Close()
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
}

func (s *Session) write(v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stdin.Encode(v) // json.Encoder já adiciona o '\n'
}

func (s *Session) notify() {
	if s.onChange != nil {
		s.onChange(s.id)
	}
}

// readLoop consome o stream-json e atualiza o estado.
func (s *Session) readLoop(r *bufio.Reader) {
	dec := json.NewDecoder(r)
	for {
		var ev map[string]any
		if err := dec.Decode(&ev); err != nil {
			s.mu.Lock()
			s.state = agui.StateEnded
			s.mu.Unlock()
			s.notify()
			return
		}
		s.handle(ev)
	}
}

func (s *Session) handle(ev map[string]any) {
	typ, _ := ev["type"].(string)
	switch typ {
	case "assistant":
		s.handleAssistant(ev)
	case "user":
		s.handleUser(ev)
	case "result":
		s.mu.Lock()
		s.state = agui.StateAwaiting // fim de turno → vez do usuário
		s.mu.Unlock()
		s.notify()
	case "control_request":
		s.handleControlRequest(ev)
	}
}

func (s *Session) handleAssistant(ev map[string]any) {
	msg, _ := ev["message"].(map[string]any)
	content, _ := msg["content"].([]any)
	s.mu.Lock()
	for _, b := range content {
		bm, _ := b.(map[string]any)
		switch bm["type"] {
		case "text":
			if t := strings.TrimSpace(asString(bm["text"])); t != "" {
				s.message = t
				s.progress = appendCapped(s.progress, t, 3)
				s.history = append(s.history, agui.HistoryLine{Role: "ai", Text: t})
			}
		case "tool_use":
			name := asString(bm["name"])
			sum := summarizeInput(bm["input"])
			s.toolCalls = append(s.toolCalls, agui.ToolCall{Name: name, Summary: sum})
			s.history = append(s.history, agui.HistoryLine{Role: "tool", Text: name + " " + sum})
		}
	}
	s.mu.Unlock()
	s.notify()
}

// handleUser captura tool_results (que chegam como eventos type=user) para o
// transcript da conversa.
func (s *Session) handleUser(ev map[string]any) {
	msg, _ := ev["message"].(map[string]any)
	content, _ := msg["content"].([]any)
	var changed bool
	s.mu.Lock()
	for _, b := range content {
		bm, _ := b.(map[string]any)
		if bm["type"] != "tool_result" {
			continue
		}
		if txt := strings.TrimSpace(toolResultText(bm["content"])); txt != "" {
			if len(txt) > 200 {
				txt = txt[:199] + "…"
			}
			s.history = append(s.history, agui.HistoryLine{Role: "tool", Text: "→ " + txt})
			changed = true
		}
	}
	s.mu.Unlock()
	if changed {
		s.notify()
	}
}

func (s *Session) handleControlRequest(ev map[string]any) {
	req, _ := ev["request"].(map[string]any)
	if asString(req["subtype"]) != "can_use_tool" {
		return
	}
	tool := asString(req["tool_name"])
	s.mu.Lock()
	s.reqID = ev["request_id"]
	s.pendingIn = req["input"]
	s.pendingTool = tool
	s.interrupt = &agui.Interrupt{
		RequestID: asString(ev["request_id"]),
		Kind:      agui.KindPermission,
		Prompt:    "Permitir " + tool + "?",
		Detail:    summarizeInput(req["input"]),
	}
	s.state = agui.StateAwaiting // precisa de você
	s.mu.Unlock()
	s.notify()
}

func asString(v any) string { s, _ := v.(string); return s }

// toolResultText extrai texto de um tool_result.content (string direta ou lista
// de blocos {type:text,text}).
func toolResultText(v any) string {
	if str, ok := v.(string); ok {
		return str
	}
	blocks, ok := v.([]any)
	if !ok {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		bm, _ := b.(map[string]any)
		if bm["type"] == "text" {
			parts = append(parts, asString(bm["text"]))
		}
	}
	return strings.Join(parts, "\n")
}

func summarizeInput(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	str := string(b)
	if len(str) > 160 {
		str = str[:159] + "…"
	}
	if str == "null" {
		return ""
	}
	return str
}

func appendCapped(xs []string, x string, n int) []string {
	xs = append(xs, x)
	if len(xs) > n {
		xs = xs[len(xs)-n:]
	}
	return xs
}
