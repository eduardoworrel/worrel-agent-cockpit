// internal/streamengine/opencode_acp.go
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

// opencodeDriver dirige uma sessão integrada via OpenCode ACP (Agent Client
// Protocol): JSON-RPC 2.0 sobre stdio de `opencode acp`.
type opencodeDriver struct{}

func (opencodeDriver) Start(ctx context.Context, sessionID, cwd string, o Opts,
	onChange func(string), persist func(role, text string)) (LiveSession, error) {
	cmd := exec.CommandContext(ctx, "opencode", "acp", "--cwd", cwd)
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
	s := &acpSession{
		id: sessionID, cmd: cmd,
		enc: json.NewEncoder(stdin), stdinW: stdin,
		onChange: onChange, persist: persist,
		state:   agui.StateWorking,
		chunks:  map[string]string{},
		pending: map[int]chan map[string]any{},
		opts:    o,
	}
	go s.readLoop(bufio.NewReaderSize(stdout, 1<<20))
	if err := s.handshake(cwd, o); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// acpSession implementa LiveSession sobre ACP.
type acpSession struct {
	id     string
	cmd    *exec.Cmd
	enc    *json.Encoder
	stdinW interface{ Close() error }
	writeFn func(any) error // nil = usa o encoder real

	onChange func(string)
	persist  func(role, text string)
	opts     Opts

	mu        sync.Mutex
	nextID    int
	acpSID    string                      // sessionId devolvido por session/new
	state     agui.State
	message   string                      // texto acumulado do turno atual
	chunks    map[string]string           // messageId → texto acumulado
	toolCalls []agui.ToolCall
	history   []agui.HistoryLine
	interrupt *agui.Interrupt
	permID    int             // id da request_permission pendente (0 = nenhuma)
	permOpts  []acpPermOption // opções da permissão pendente
	pending   map[int]chan map[string]any // id → canal de resposta (requests do cliente)
}

type acpPermOption struct {
	OptionID string
	Kind     string
}

func (s *acpSession) Snapshot() agui.Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return agui.Snapshot{
		SessionID: s.id,
		State:     s.state,
		Message:   s.message,
		ToolCalls: append([]agui.ToolCall(nil), s.toolCalls...),
		History:   append([]agui.HistoryLine(nil), s.history...),
		Interrupt: s.interrupt,
	}
}

// handshake faz initialize + session/new (com MCP, se houver) e injeta a memória
// como primeiro contexto, se Opts.SystemAppend != "".
func (s *acpSession) handshake(cwd string, o Opts) error {
	if _, err := s.call("initialize", map[string]any{
		"protocolVersion": 1,
		"clientCapabilities": map[string]any{
			"fs": map[string]any{"readTextFile": false, "writeTextFile": false},
		},
	}); err != nil {
		return err
	}
	mcp := []any{}
	if o.MCPURL != "" {
		mcp = append(mcp, map[string]any{
			"name": "worrel", "type": "http", "url": o.MCPURL,
		})
	}
	res, err := s.call("session/new", map[string]any{"cwd": cwd, "mcpServers": mcp})
	if err != nil {
		return err
	}
	sid, _ := res["sessionId"].(string)
	if sid == "" {
		return fmt.Errorf("session/new sem sessionId")
	}
	s.mu.Lock()
	s.acpSID = sid
	s.mu.Unlock()
	return nil
}

// SendPrompt manda um turno do usuário. A memória (SystemAppend) entra como um
// bloco de texto prefixado SOMENTE no primeiro turno (ACP não tem system-prompt
// em session/new; promptCapabilities.embeddedContext garante o bloco de texto).
func (s *acpSession) SendPrompt(text string) error {
	line := agui.HistoryLine{Role: "you", Text: text}
	s.mu.Lock()
	first := len(s.history) == 0
	sysAppend := s.opts.SystemAppend
	s.state = agui.StateWorking
	s.message = ""
	s.toolCalls = nil
	s.history = append(s.history, line)
	sid := s.acpSID
	s.mu.Unlock()
	s.persistLine(line)
	s.notify()

	blocks := []map[string]any{}
	if first && strings.TrimSpace(sysAppend) != "" {
		blocks = append(blocks, map[string]any{"type": "text", "text": sysAppend})
	}
	blocks = append(blocks, map[string]any{"type": "text", "text": text})

	go func() {
		res, err := s.call("session/prompt", map[string]any{"sessionId": sid, "prompt": blocks})
		if err != nil {
			s.onPromptResult("error")
			return
		}
		stop, _ := res["stopReason"].(string)
		s.onPromptResult(stop)
	}()
	return nil
}

// onPromptResult trata o fim de turno (resultado de session/prompt).
func (s *acpSession) onPromptResult(stopReason string) {
	s.mu.Lock()
	msg := s.message
	var line agui.HistoryLine
	if strings.TrimSpace(msg) != "" {
		line = agui.HistoryLine{Role: "ai", Text: msg}
		s.history = append(s.history, line)
	}
	s.state = agui.StateAwaiting
	s.mu.Unlock()
	if line.Text != "" {
		s.persistLine(line)
	}
	s.notify()
}

func (s *acpSession) Close() {
	s.mu.Lock()
	_ = s.stdinW.Close()
	s.mu.Unlock()
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
}

// Respond responde a session/request_permission selecionando a opção allow/reject.
func (s *acpSession) Respond(allow bool) error {
	s.mu.Lock()
	if s.permID == 0 {
		s.mu.Unlock()
		return fmt.Errorf("nenhuma permissão pendente")
	}
	id := s.permID
	opts := s.permOpts
	s.interrupt = nil
	s.permID = 0
	s.permOpts = nil
	if allow {
		s.state = agui.StateWorking
	}
	s.mu.Unlock()
	s.notify()
	pick := selectPermOption(opts, allow)
	return s.write(map[string]any{
		"jsonrpc": "2.0", "id": id,
		"result": map[string]any{
			"outcome": map[string]any{"outcome": "selected", "optionId": pick},
		},
	})
}

// selectPermOption escolhe o optionId allow/reject pelo `kind` (allow_once /
// reject_once), com fallback por substring no id e, por fim, a 1ª/última opção.
func selectPermOption(opts []acpPermOption, allow bool) string {
	wantKind := "reject_once"
	wantSub := "reject"
	if allow {
		wantKind, wantSub = "allow_once", "allow"
	}
	for _, o := range opts {
		if o.Kind == wantKind {
			return o.OptionID
		}
	}
	for _, o := range opts {
		if strings.Contains(strings.ToLower(o.OptionID), wantSub) {
			return o.OptionID
		}
	}
	if len(opts) == 0 {
		return ""
	}
	if allow {
		return opts[0].OptionID
	}
	return opts[len(opts)-1].OptionID
}

// --- wire JSON-RPC ---

func (s *acpSession) write(v any) error {
	if s.writeFn != nil {
		return s.writeFn(v)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enc.Encode(v)
}

// call envia uma request JSON-RPC e bloqueia até a resposta correspondente.
func (s *acpSession) call(method string, params any) (map[string]any, error) {
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	ch := make(chan map[string]any, 1)
	s.pending[id] = ch
	s.mu.Unlock()
	if err := s.write(map[string]any{
		"jsonrpc": "2.0", "id": id, "method": method, "params": params,
	}); err != nil {
		return nil, err
	}
	resp, ok := <-ch
	if !ok {
		return nil, fmt.Errorf("acp: conexão encerrada")
	}
	if e, ok := resp["error"].(map[string]any); ok {
		return nil, fmt.Errorf("acp %s: %v", method, e["message"])
	}
	out, _ := resp["result"].(map[string]any)
	return out, nil
}

// drainPending closes every pending response channel and resets the map.
// Must NOT be called while holding s.mu.
func (s *acpSession) drainPending() {
	s.mu.Lock()
	for _, ch := range s.pending {
		close(ch)
	}
	s.pending = map[int]chan map[string]any{}
	s.mu.Unlock()
}

func (s *acpSession) readLoop(r *bufio.Reader) {
	dec := json.NewDecoder(r)
	for {
		var msg map[string]any
		if err := dec.Decode(&msg); err != nil {
			s.mu.Lock()
			s.state = agui.StateEnded
			s.mu.Unlock()
			s.drainPending()
			s.notify()
			return
		}
		// resposta a uma request nossa?
		if idF, ok := msg["id"].(float64); ok && msg["method"] == nil {
			s.mu.Lock()
			ch := s.pending[int(idF)]
			delete(s.pending, int(idF))
			s.mu.Unlock()
			if ch != nil {
				ch <- msg
			}
			continue
		}
		// notificação / request do servidor
		switch msg["method"] {
		case "session/update":
			if p, ok := msg["params"].(map[string]any); ok {
				if u, ok := p["update"].(map[string]any); ok {
					s.handleUpdate(u)
				}
			}
		case "session/request_permission":
			s.handlePermissionRequest(msg) // Task 6
		}
	}
}

// handleUpdate mapeia um session/update.update no estado AG-UI.
func (s *acpSession) handleUpdate(u map[string]any) {
	switch u["sessionUpdate"] {
	case "agent_message_chunk":
		s.appendChunk(u)
	case "agent_thought_chunk":
		// reasoning não vira mensagem (espelha o Claude, que ignora thinking).
	case "tool_call", "tool_call_update":
		s.handleToolCall(u) // Task 6
	}
}

func (s *acpSession) appendChunk(u map[string]any) {
	mid, _ := u["messageId"].(string)
	content, _ := u["content"].(map[string]any)
	text, _ := content["text"].(string)
	if text == "" {
		return
	}
	s.mu.Lock()
	s.chunks[mid] += text
	full := s.chunks[mid]
	s.message = full
	s.mu.Unlock()
	s.notify()
}

func (s *acpSession) notify() {
	if s.onChange != nil {
		s.onChange(s.id)
	}
}

func (s *acpSession) persistLine(l agui.HistoryLine) {
	if s.persist != nil {
		s.persist(l.Role, l.Text)
	}
}

func (s *acpSession) handleToolCall(u map[string]any) {
	name, _ := u["title"].(string)
	if name == "" {
		name, _ = u["toolCallId"].(string)
	}
	sum := summarizeInput(u["rawInput"])
	line := agui.HistoryLine{Role: "tool", Text: strings.TrimSpace(name + " " + sum)}
	s.mu.Lock()
	// tool_call_update do mesmo id não duplica: só registramos no "tool_call".
	if u["sessionUpdate"] == "tool_call" {
		s.toolCalls = append(s.toolCalls, agui.ToolCall{Name: name, Summary: sum})
		s.history = append(s.history, line)
	}
	s.mu.Unlock()
	if u["sessionUpdate"] == "tool_call" {
		s.persistLine(line)
	}
	s.notify()
}

func (s *acpSession) handlePermissionRequest(msg map[string]any) {
	idF, _ := msg["id"].(float64)
	params, _ := msg["params"].(map[string]any)
	tc, _ := params["toolCall"].(map[string]any)
	tool, _ := tc["title"].(string)
	var opts []acpPermOption
	if raw, ok := params["options"].([]any); ok {
		for _, o := range raw {
			om, _ := o.(map[string]any)
			opts = append(opts, acpPermOption{
				OptionID: asString(om["optionId"]),
				Kind:     asString(om["kind"]),
			})
		}
	}
	s.mu.Lock()
	s.permID = int(idF)
	s.permOpts = opts
	s.interrupt = &agui.Interrupt{
		RequestID: fmt.Sprintf("%d", int(idF)),
		Kind:      agui.KindPermission,
		Prompt:    "Permitir " + tool + "?",
		Detail:    summarizeInput(tc["rawInput"]),
	}
	s.state = agui.StateAwaiting
	s.mu.Unlock()
	s.notify()
}
