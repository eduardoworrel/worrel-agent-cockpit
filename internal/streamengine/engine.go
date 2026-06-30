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
	// persist grava cada linha do histórico no store (durável), para que o chat
	// sobreviva ao restart do app. Pode ser nil (sessão efêmera).
	persist func(role, text string)

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

// Opts configura uma sessão do motor.
type Opts struct {
	Mode         PermissionMode // modo de permissão ("auto" se vazio)
	SystemAppend string         // memória injetada no início (--append-system-prompt)
	MCPURL       string         // MCP do worrel p/ a sessão CONSULTAR a memória sob demanda
}

// claudeArgs são os flags provados: stream-json bidirecional + permissão via stdio.
func claudeArgs(o Opts) []string {
	mode := o.Mode
	if mode == "" {
		mode = "auto"
	}
	args := []string{
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
	if o.SystemAppend != "" {
		args = append(args, "--append-system-prompt", o.SystemAppend)
	}
	if o.MCPURL != "" {
		args = append(args, "--mcp-config", mcpConfigJSON(o.MCPURL))
	}
	return args
}

// mcpConfigJSON é o JSON inline do --mcp-config (servidor worrel via http stream).
func mcpConfigJSON(url string) string {
	return `{"mcpServers":{"worrel":{"type":"http","url":"` + url + `"}}}`
}

// Start spawna o claude no cwd com as opções dadas e começa a ler o stream.
// onChange é notificado a cada transição (pode ser nil).
func Start(ctx context.Context, sessionID, cwd string, o Opts, onChange func(string), persist func(role, text string)) (*Session, error) {
	cmd := exec.CommandContext(ctx, "claude", claudeArgs(o)...)
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
		onChange: onChange, persist: persist, state: agui.StateWorking,
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
	line := agui.HistoryLine{Role: "you", Text: text}
	s.mu.Lock()
	s.state = agui.StateWorking
	s.toolCalls = nil
	s.history = append(s.history, line)
	s.mu.Unlock()
	s.persistLines(line)
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
	if reqID == nil {
		// Nada pendente (já resolvido em outra aba, órfão, ou snapshot defasado):
		// responder a nada é IDEMPOTENTE, não erro. Garante o interrupt limpo e sai
		// — sem 409 e sem linha de histórico falsa. O front re-sincroniza e os botões
		// somem (antes virava 409 e o clique "não disparava nada").
		s.interrupt = nil
		s.mu.Unlock()
		s.notify()
		return nil
	}
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
	sysLine := agui.HistoryLine{Role: "system", Text: "você " + decision + " " + tool}
	s.history = append(s.history, sysLine)
	s.mu.Unlock()
	s.persistLines(sysLine)
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

// persistLines grava as linhas dadas no store (durável), fora do lock. É o que
// permite ao chat sobreviver ao restart: na volta, a borda HTTP reconstrói o
// histórico a partir desses eventos quando a sessão não está mais viva na memória.
func (s *Session) persistLines(lines ...agui.HistoryLine) {
	if s.persist == nil {
		return
	}
	for _, l := range lines {
		s.persist(l.Role, l.Text)
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
	var added []agui.HistoryLine
	s.mu.Lock()
	for _, b := range content {
		bm, _ := b.(map[string]any)
		switch bm["type"] {
		case "text":
			if t := strings.TrimSpace(asString(bm["text"])); t != "" {
				s.message = t
				// NÃO vira progress: o card mostra EVENTOS NARRADOS (gerados pelo
				// summarizer), não as mensagens cruas. Aqui só guardamos o histórico.
				line := agui.HistoryLine{Role: "ai", Text: t}
				s.history = append(s.history, line)
				added = append(added, line)
			}
		case "tool_use":
			name := asString(bm["name"])
			sum := summarizeInput(bm["input"])
			s.toolCalls = append(s.toolCalls, agui.ToolCall{Name: name, Summary: sum})
			line := agui.HistoryLine{Role: "tool", Text: name + " " + sum}
			s.history = append(s.history, line)
			added = append(added, line)
		}
	}
	s.mu.Unlock()
	s.persistLines(added...)
	s.notify()
}

// handleUser captura tool_results (que chegam como eventos type=user) para o
// transcript da conversa.
func (s *Session) handleUser(ev map[string]any) {
	msg, _ := ev["message"].(map[string]any)
	content, _ := msg["content"].([]any)
	var added []agui.HistoryLine
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
			line := agui.HistoryLine{Role: "tool", Text: "→ " + txt}
			s.history = append(s.history, line)
			added = append(added, line)
		}
	}
	s.mu.Unlock()
	if len(added) > 0 {
		s.persistLines(added...)
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

