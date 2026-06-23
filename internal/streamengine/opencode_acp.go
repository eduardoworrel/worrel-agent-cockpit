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
	s.state = agui.StateAwaiting
	s.mu.Unlock()
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

// Respond é stub nesta task (só compila); a Task 6 implementa de verdade.
func (s *acpSession) Respond(allow bool) error { return nil }

// --- wire JSON-RPC ---

func (s *acpSession) write(v any) error {
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

// handleToolCall e handlePermissionRequest: stubs preenchidos na Task 6.
func (s *acpSession) handleToolCall(u map[string]any)         {}
func (s *acpSession) handlePermissionRequest(msg map[string]any) {}
