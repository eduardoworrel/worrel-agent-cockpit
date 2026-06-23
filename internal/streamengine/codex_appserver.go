// internal/streamengine/codex_appserver.go
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

// codexDriver dirige uma sessão integrada via `codex app-server` (JSON-RPC 2.0
// sobre stdio). Protocolo verificado no spike 2026-06-23-codex-app-server.md.
type codexDriver struct{}

func (codexDriver) Start(ctx context.Context, sessionID, cwd string, o Opts,
	onChange func(string), persist func(role, text string)) (LiveSession, error) {
	args := []string{"app-server"}
	if o.MCPURL != "" {
		args = append(args, "-c", "experimental_use_rmcp_client=true",
			"-c", `mcp_servers.worrel.url="`+o.MCPURL+`"`)
	}
	cmd := exec.CommandContext(ctx, "codex", args...)
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
	s := &codexSession{
		id: sessionID, cmd: cmd,
		enc: json.NewEncoder(stdin), stdinW: stdin,
		onChange: onChange, persist: persist, opts: o, cwd: cwd,
		state:   agui.StateWorking,
		deltas:  map[string]string{},
		pending: map[int]chan map[string]any{},
	}
	go s.readLoop(bufio.NewReaderSize(stdout, 1<<20))
	if err := s.handshake(cwd); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

type codexSession struct {
	id     string
	cmd    *exec.Cmd
	enc    *json.Encoder
	stdinW interface{ Close() error }
	writeFn func(any) error // test-indirection; nil = encoder real

	onChange func(string)
	persist  func(role, text string)
	opts     Opts
	cwd      string

	mu        sync.Mutex
	nextID    int
	threadID  string
	state     agui.State
	message   string
	deltas    map[string]string // itemId → texto acumulado do agentMessage
	toolCalls []agui.ToolCall
	history   []agui.HistoryLine
	pending   map[int]chan map[string]any
}

func (s *codexSession) Snapshot() agui.Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return agui.Snapshot{
		SessionID: s.id, State: s.state, Message: s.message,
		ToolCalls: append([]agui.ToolCall(nil), s.toolCalls...),
		History:   append([]agui.HistoryLine(nil), s.history...),
	}
}

func (s *codexSession) handshake(cwd string) error {
	if _, err := s.call("initialize", map[string]any{
		"clientInfo": map[string]any{"name": "worrel", "version": "0.1"},
	}); err != nil {
		return err
	}
	res, err := s.call("thread/start", map[string]any{"cwd": cwd})
	if err != nil {
		return err
	}
	thread, _ := res["thread"].(map[string]any)
	tid, _ := thread["id"].(string)
	if tid == "" {
		return fmt.Errorf("thread/start sem id")
	}
	s.mu.Lock()
	s.threadID = tid
	s.mu.Unlock()
	return nil
}

func (s *codexSession) SendPrompt(text string) error {
	line := agui.HistoryLine{Role: "you", Text: text}
	s.mu.Lock()
	first := len(s.history) == 0
	sysAppend := s.opts.SystemAppend
	s.state = agui.StateWorking
	s.message = ""
	s.toolCalls = nil
	s.history = append(s.history, line)
	tid := s.threadID
	cwd := s.cwd
	s.mu.Unlock()
	s.persistLineCodex(line)
	s.notify()

	input := []map[string]any{}
	if first && strings.TrimSpace(sysAppend) != "" {
		input = append(input, map[string]any{"type": "text", "text": sysAppend})
	}
	input = append(input, map[string]any{"type": "text", "text": text})

	go func() {
		_, err := s.call("turn/start", map[string]any{
			"threadId": tid, "cwd": cwd, "approvalPolicy": "never", "input": input,
		})
		if err != nil {
			s.finishTurn() // libera o turno mesmo em erro
		}
		// fim de turno REAL chega por notif turn/completed (ver handleNotification)
	}()
	return nil
}

// finishTurn fecha o turno: flush da mensagem acumulada para o histórico + persist.
func (s *codexSession) finishTurn() {
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
		s.persistLineCodex(line)
	}
	s.notify()
}

func (s *codexSession) Respond(allow bool) error { return nil } // codex auto-aprova (approvalPolicy=never)

func (s *codexSession) Close() {
	s.mu.Lock()
	_ = s.stdinW.Close()
	s.mu.Unlock()
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
}

// --- wire JSON-RPC (mesmo padrão do opencode_acp.go) ---

func (s *codexSession) write(v any) error {
	if s.writeFn != nil {
		return s.writeFn(v)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enc.Encode(v)
}

func (s *codexSession) call(method string, params any) (map[string]any, error) {
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
		return nil, fmt.Errorf("codex: conexão encerrada")
	}
	if e, ok := resp["error"].(map[string]any); ok {
		return nil, fmt.Errorf("codex %s: %v", method, e["message"])
	}
	out, _ := resp["result"].(map[string]any)
	return out, nil
}

func (s *codexSession) drainPending() {
	s.mu.Lock()
	for _, ch := range s.pending {
		close(ch)
	}
	s.pending = map[int]chan map[string]any{}
	s.mu.Unlock()
}

func (s *codexSession) readLoop(r *bufio.Reader) {
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
		if m, ok := msg["method"].(string); ok {
			params, _ := msg["params"].(map[string]any)
			s.handleNotification(m, params)
		}
	}
}

// handleNotification mapeia uma notificação do codex app-server no estado AG-UI.
func (s *codexSession) handleNotification(method string, params map[string]any) {
	switch method {
	case "item/agentMessage/delta":
		s.appendDelta(params)
	case "turn/completed":
		s.finishTurn()
	case "item/started", "item/completed":
		s.handleItem(method, params) // Task 2 preenche
	}
}

func (s *codexSession) appendDelta(params map[string]any) {
	itemID, _ := params["itemId"].(string)
	delta, _ := params["delta"].(string)
	if delta == "" {
		return
	}
	s.mu.Lock()
	s.deltas[itemID] += delta
	s.message = s.deltas[itemID]
	s.mu.Unlock()
	s.notify()
}

func (s *codexSession) notify() {
	if s.onChange != nil {
		s.onChange(s.id)
	}
}

func (s *codexSession) persistLineCodex(l agui.HistoryLine) {
	if s.persist != nil {
		s.persist(l.Role, l.Text)
	}
}

// handleItem: registra ToolCall + history para itens tool (ex: commandExecution).
// Só registra em item/started; item/completed reutiliza o mesmo id e não duplica.
func (s *codexSession) handleItem(method string, params map[string]any) {
	if method != "item/started" {
		return // só registramos no started; completed reusa o mesmo id
	}
	item, _ := params["item"].(map[string]any)
	typ, _ := item["type"].(string)
	if typ == "" || typ == "userMessage" {
		return // echo do próprio usuário não é tool call
	}
	cmd, _ := item["command"].(string)
	sum := cmd
	if sum == "" {
		sum = summarizeInput(item)
	}
	line := agui.HistoryLine{Role: "tool", Text: strings.TrimSpace(typ + " " + sum)}
	s.mu.Lock()
	s.toolCalls = append(s.toolCalls, agui.ToolCall{Name: typ, Summary: sum})
	s.history = append(s.history, line)
	s.mu.Unlock()
	s.persistLineCodex(line)
	s.notify()
}
