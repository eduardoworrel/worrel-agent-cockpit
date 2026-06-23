# Codex Integrated Driver (codex app-server) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Add a third Integrated-mode provider — Codex — behind the existing `Driver`/`LiveSession` seam, speaking the `codex app-server` JSON-RPC protocol, so selecting Codex + Integrated drives a real Codex session instead of returning HTTP 500.

**Architecture:** A new `codexDriver` in `internal/streamengine` spawns `codex app-server` (stdio JSON-RPC 2.0) and a `codexSession` implementing `LiveSession`, structurally mirroring the existing `opencode_acp.go` wire layer (call/readLoop/drainPending). It maps codex notifications (`item/agentMessage/delta`, `item/started`, `turn/completed`) onto the same `agui.Snapshot` contract. Registering it in `DefaultDrivers()` closes the current codex→500 gap automatically.

**Tech Stack:** Go 1.26, hand-rolled JSON-RPC over stdio (no new deps), `agui` snapshot contract.

## Global Constraints

- Go module `github.com/eduardoworrel/worrel-agent-cockpit`; Go 1.26.1. No new third-party Go deps.
- Provider id is `"codex"` (the codex adapter's `ID()`).
- Claude and OpenCode drivers untouched.
- Binary invocation: `codex app-server` (+ optional `-c experimental_use_rmcp_client=true -c mcp_servers.worrel.url="<url>"` when MCP is requested). Newline-delimited JSON-RPC 2.0 over stdio.
- VERIFIED protocol facts (codex v0.139.0, spike `docs/superpowers/spikes/2026-06-23-codex-app-server.md`):
  - `initialize` params `{clientInfo:{name,version}}` → result `{userAgent, codexHome, ...}`.
  - `thread/start` params `{cwd}` → result `{thread:{id}}`.
  - `turn/start` params `{threadId, cwd, approvalPolicy:"never", input:[{type:"text",text}]}` → returns immediately `{turn:{status:"inProgress"}}`.
  - notif `item/agentMessage/delta` `{itemId, delta}` → assistant text, accumulate by `itemId`.
  - notif `item/started`/`item/completed` `{item:{type,...}}`: `item.type=="userMessage"` is our own echo (ignore); `item.type=="commandExecution"` has `{id, command, status}` (a tool call).
  - notif `turn/completed` `{turn:{status:"completed"}}` → end of turn.
  - Permission: codex auto-approves with `approvalPolicy:"never"`; no permission round-trip in v1 (Respond is a no-op).

---

## File Structure

- `internal/streamengine/codex_appserver.go` (**new**) — `codexDriver`, `codexSession` (LiveSession), JSON-RPC wire layer, notification→snapshot mapping.
- `internal/streamengine/codex_appserver_test.go` (**new**) — unit tests driving `handleNotification` with canned maps.
- `internal/streamengine/codex_live_test.go` (**new**) — `//go:build integration` live round-trip.
- `internal/streamengine/driver.go` (**modify**) — register `"codex": codexDriver{}` in `DefaultDrivers()`.

---

## Task 1: codexSession happy path (text turn) + register driver

**Files:**
- Create: `internal/streamengine/codex_appserver.go`
- Create: `internal/streamengine/codex_appserver_test.go`
- Modify: `internal/streamengine/driver.go` (add codex to DefaultDrivers)

**Interfaces:**
- Consumes: `LiveSession`, `Driver`, `Opts`, `agui.*`, existing helpers `asString`, `summarizeInput`.
- Produces:
  - `type codexDriver struct{}` implementing `Driver`.
  - `type codexSession struct{ ... }` implementing `LiveSession`.
  - `func (s *codexSession) handleNotification(method string, params map[string]any)` — maps one codex notification onto state. **Task 2 extends it for items.**
  - `Snapshot/SendPrompt/Respond/Close` per `LiveSession`.

- [ ] **Step 1: Write the failing test**

```go
// internal/streamengine/codex_appserver_test.go
package streamengine

import (
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/agui"
)

func newTestCodex() *codexSession {
	return &codexSession{id: "s1", state: agui.StateWorking, deltas: map[string]string{},
		pending: map[int]chan map[string]any{}}
}

func TestCodexAccumulatesAgentDeltas(t *testing.T) {
	s := newTestCodex()
	s.handleNotification("item/agentMessage/delta", map[string]any{"itemId": "i1", "delta": "o"})
	s.handleNotification("item/agentMessage/delta", map[string]any{"itemId": "i1", "delta": "k"})
	if got := s.Snapshot().Message; got != "ok" {
		t.Fatalf("Message = %q, quer ok", got)
	}
}

func TestCodexTurnCompletedSetsAwaiting(t *testing.T) {
	s := newTestCodex()
	s.handleNotification("turn/completed", map[string]any{"turn": map[string]any{"status": "completed"}})
	if got := s.Snapshot().State; got != agui.StateAwaiting {
		t.Fatalf("State = %q, quer awaiting", got)
	}
}

func TestCodexUserMessageEchoIgnored(t *testing.T) {
	s := newTestCodex()
	s.handleNotification("item/started", map[string]any{
		"item": map[string]any{"type": "userMessage",
			"content": []any{map[string]any{"type": "text", "text": "hi"}}}})
	if got := s.Snapshot().Message; got != "" {
		t.Fatalf("echo do usuário não deveria virar Message, veio %q", got)
	}
}

func TestCodexDriverSatisfiesDriver(t *testing.T) {
	var _ Driver = codexDriver{}
	var _ LiveSession = (*codexSession)(nil)
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/streamengine/ -run Codex -v`
Expected: FAIL — `undefined: codexSession`, `codexDriver`.

- [ ] **Step 3: Implement the driver + wire layer**

Create `internal/streamengine/codex_appserver.go`. Structure mirrors `opencode_acp.go` (same call/readLoop/drainPending/write/notify/persistLine helpers — reuse the same patterns and lock discipline). The codex-specific parts:

```go
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

	mu       sync.Mutex
	nextID   int
	threadID string
	state    agui.State
	message  string
	deltas   map[string]string // itemId → texto acumulado do agentMessage
	toolCalls []agui.ToolCall
	history  []agui.HistoryLine
	pending  map[int]chan map[string]any
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

// handleItem: stub preenchido na Task 2.
func (s *codexSession) handleItem(method string, params map[string]any) {}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/streamengine/ -run Codex -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Register the driver**

In `internal/streamengine/driver.go`, add codex to `DefaultDrivers()`:

```go
func DefaultDrivers() map[string]Driver {
	return map[string]Driver{
		"claude-code": claudeDriver{},
		"opencode":    opencodeDriver{},
		"codex":       codexDriver{},
	}
}
```

- [ ] **Step 6: Build + full package tests**

Run: `go build ./... && go test ./internal/streamengine/`
Expected: build OK; all PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/streamengine/codex_appserver.go internal/streamengine/codex_appserver_test.go internal/streamengine/driver.go
git commit -m "feat(engine): Codex app-server driver — text-turn happy path + register"
```

---

## Task 2: Codex tool items (commandExecution) + message already persisted

**Files:**
- Modify: `internal/streamengine/codex_appserver.go` (`handleItem`)
- Test: `internal/streamengine/codex_appserver_test.go` (append)

**Interfaces:**
- Consumes: verified `commandExecution` item shape `{type:"commandExecution", id, command, status}`.
- Produces: `handleItem` records a `agui.ToolCall` + history line on `item/started` for tool items (type != userMessage), ignoring `userMessage` echoes and not double-recording on `item/completed`.

- [ ] **Step 1: Write the failing tests**

```go
// append to internal/streamengine/codex_appserver_test.go
func TestCodexCommandItemRecorded(t *testing.T) {
	s := newTestCodex()
	s.handleNotification("item/started", map[string]any{
		"item": map[string]any{"type": "commandExecution", "id": "call_1",
			"command": "/bin/zsh -lc 'echo hi'", "status": "inProgress"}})
	tcs := s.Snapshot().ToolCalls
	if len(tcs) != 1 || tcs[0].Name != "commandExecution" {
		t.Fatalf("ToolCalls = %+v, quer 1 commandExecution", tcs)
	}
}

func TestCodexCompletedDoesNotDoubleRecord(t *testing.T) {
	s := newTestCodex()
	item := map[string]any{"item": map[string]any{"type": "commandExecution", "id": "call_1",
		"command": "echo hi", "status": "completed"}}
	s.handleNotification("item/started", item)
	s.handleNotification("item/completed", item)
	if n := len(s.Snapshot().ToolCalls); n != 1 {
		t.Fatalf("ToolCalls = %d, quer 1 (sem duplicar no completed)", n)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/streamengine/ -run 'CodexCommand|CodexCompleted' -v`
Expected: FAIL — ToolCalls empty (handleItem is a stub).

- [ ] **Step 3: Implement handleItem**

```go
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
```

- [ ] **Step 4: Run to verify they pass**

Run: `go test ./internal/streamengine/ -run Codex -v`
Expected: PASS (all codex tests).

- [ ] **Step 5: Build + tests + commit**

Run: `go build ./... && go test ./internal/streamengine/`

```bash
git add internal/streamengine/codex_appserver.go internal/streamengine/codex_appserver_test.go
git commit -m "feat(engine): Codex tool items (commandExecution) → ToolCall + history"
```

---

## Task 3: Codex live integration test (gated)

**Files:**
- Create: `internal/streamengine/codex_live_test.go`

- [ ] **Step 1: Write the gated live test**

```go
//go:build integration

package streamengine

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/agui"
)

func TestCodexAppServerLive(t *testing.T) {
	if _, err := exec.LookPath("codex"); err != nil {
		t.Skip("codex não instalado")
	}
	done := make(chan struct{}, 8)
	onChange := func(string) { select { case done <- struct{}{}: default: } }
	sess, err := codexDriver{}.Start(context.Background(), "live1", t.TempDir(), Opts{}, onChange, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if err := sess.SendPrompt("responda apenas a palavra ok"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(90 * time.Second)
	for {
		select {
		case <-done:
			snap := sess.Snapshot()
			if snap.State == agui.StateAwaiting && snap.Message != "" {
				return
			}
		case <-deadline:
			t.Fatalf("timeout; snapshot: %+v", sess.Snapshot())
		}
	}
}
```

- [ ] **Step 2: Confirm default exclusion**

Run: `go test ./internal/streamengine/`
Expected: PASS, live test NOT run.

- [ ] **Step 3: Run live for real**

Run: `go test -tags integration ./internal/streamengine/ -run TestCodexAppServerLive -v -timeout 120s`
Expected: PASS — real `codex app-server` returns text and completes the turn. If it FAILS, the notification mapping is wrong: report details, do NOT weaken the test.

- [ ] **Step 4: Commit**

```bash
git add internal/streamengine/codex_live_test.go
git commit -m "test(engine): gated live codex app-server round-trip"
```

---

## Self-Review Notes

- **Gap closed:** registering `"codex"` in DefaultDrivers (Task 1 Step 5) makes the handler's `normalizeProvider("codex")→("codex",true)` route to a real driver — the prior codex→`ErrUnknownProvider`→HTTP 500 is gone. No UI change needed (codex was never UI-blocked, only antigravity).
- **Permission:** v1 uses `approvalPolicy:"never"` (auto-approve), matching the opencode decision; `Respond` is a no-op. A guardian-approval round-trip is a later spike if wanted.
- **Concurrency:** the wire layer is copied from the reviewed-and-fixed `opencode_acp.go` (drain pending on EOF, Close serialized with write, buffered pending channels). Keep those invariants.
- **MCP/memory:** MCP via `-c` overrides (same as the codex adapter); memory prepended to first turn input (no system-prompt field), mirroring opencode.
- **Out of scope:** guardian approvals, file-change items beyond generic tool recording, token-usage→ContextMeasured.
</content>
