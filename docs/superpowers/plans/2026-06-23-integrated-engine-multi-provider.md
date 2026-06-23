# Integrated Engine — Multi-Provider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the "Integrado" session mode (stream-json engine, no PTY) work with providers other than Claude — starting with OpenCode via the Agent Client Protocol (ACP) — instead of being hardcoded to the `claude` binary.

**Architecture:** Extract a provider-agnostic `Driver` seam inside `internal/streamengine`. The current Claude implementation becomes `claudeDriver` (zero behavior change). `Manager` gains a driver registry keyed by provider id and `Start` takes a provider. A new `opencodeDriver` speaks ACP (JSON-RPC 2.0 over stdio) and maps ACP events onto the same `agui.Snapshot` contract. The HTTP handler `POST /api/sessions/engine` and the web wizard pass the selected provider through; `agy` is explicitly blocked from Integrated mode (no stream protocol exists).

**Tech Stack:** Go 1.26 (backend), React + TypeScript + Vite (`web/`), JSON-RPC 2.0 over stdio (ACP), `agui` snapshot contract.

## Global Constraints

- Go module: `github.com/eduardoworrel/worrel-agent-cockpit`; Go 1.26.1.
- The Claude integrated path MUST remain byte-for-byte behavior-identical after the refactor (it is the reference implementation and is in daily use).
- No new third-party Go dependencies — implement the minimal ACP JSON-RPC client by hand with `encoding/json` + `os/exec`, mirroring the existing `streamengine` style.
- Provider ids are the adapter `ID()` strings already in use: `"claude-code"`, `"opencode"`, `"codex"`, `"antigravity"`.
- `agy` (antigravity) has NO stream protocol (`agy` rejects `--output-format`); it is unsupported for Integrated and must be blocked in the UI, not silently routed to Claude.
- All Go tests run with `go test ./...` from repo root; web typecheck via `npm run build` in `web/`.
- ACP binary invocation: `opencode acp --cwd <dir>` (stdio JSON-RPC, newline-delimited).
- Verified ACP facts (opencode v1.17.9): `initialize` → `agentCapabilities.mcpCapabilities.http=true`; `session/new {cwd, mcpServers}` → `{sessionId}`; `session/prompt {sessionId, prompt:[{type:"text",text}]}` → `{stopReason:"end_turn"}`; streaming via `session/update` notifications with `update.sessionUpdate ∈ {agent_message_chunk, agent_thought_chunk, tool_call, tool_call_update, usage_update, available_commands_update}`; `agent_message_chunk.content = {type:"text", text:"..."}`.

---

## File Structure

- `internal/streamengine/driver.go` (**new**) — the `Driver` and `LiveSession` interfaces; the driver registry helper.
- `internal/streamengine/engine.go` (**modify**) — `Session` (rename receiver methods stay) becomes the Claude `LiveSession`; add `claudeDriver` implementing `Driver`.
- `internal/streamengine/manager.go` (**modify**) — store `LiveSession` (interface) instead of concrete `*Session`; `Start` takes `provider string`; hold a `map[string]Driver`.
- `internal/streamengine/opencode_acp.go` (**new**) — the ACP JSON-RPC client + `opencodeDriver` + its `LiveSession`.
- `internal/streamengine/opencode_acp_test.go` (**new**) — unit tests for ACP event→snapshot mapping (no real process; feed canned JSON-RPC lines).
- `internal/streamengine/driver_test.go` (**new**) — registry + provider-routing tests.
- `internal/httpapi/engine_session.go` (**modify**) — decode `provider` from request, reject `antigravity`, pass provider to `Engine.Start`.
- `web/src/api.ts` (**modify**) — `createEngineSession` sends `provider`.
- `web/src/components/NewSessionWizard.tsx` (**modify**) — pass `adapterId`; disable Integrated for `antigravity`.
- `web/src/i18n.ts` (**modify**) — copy for the "Integrated unavailable for this provider" hint (PT + EN).

---

## Task 1: Extract the `Driver` / `LiveSession` seam (no behavior change)

**Files:**
- Create: `internal/streamengine/driver.go`
- Modify: `internal/streamengine/manager.go`
- Modify: `internal/streamengine/engine.go:96-120` (wrap `Start` in `claudeDriver`)
- Test: `internal/streamengine/driver_test.go`

**Interfaces:**
- Produces:
  - `type LiveSession interface { Snapshot() agui.Snapshot; SendPrompt(text string) error; Respond(allow bool) error; Close() }`
  - `type Driver interface { Start(ctx context.Context, sessionID, cwd string, o Opts, onChange func(string), persist func(role, text string)) (LiveSession, error) }`
  - `func DefaultDrivers() map[string]Driver` — returns `{"claude-code": claudeDriver{}}` for now.
- Consumes: existing `agui.Snapshot`, `Opts`, and the package `Start(...)` function.

- [ ] **Step 1: Write the failing test**

```go
// internal/streamengine/driver_test.go
package streamengine

import (
	"context"
	"testing"
)

func TestDefaultDriversHasClaude(t *testing.T) {
	d := DefaultDrivers()
	if _, ok := d["claude-code"]; !ok {
		t.Fatalf("DefaultDrivers() faltou claude-code: %v", keys(d))
	}
}

func TestClaudeDriverSatisfiesDriver(t *testing.T) {
	var _ Driver = claudeDriver{}
}

// compile-time guard that the concrete Claude session satisfies LiveSession.
func TestSessionSatisfiesLiveSession(t *testing.T) {
	var _ LiveSession = (*Session)(nil)
}

func keys(m map[string]Driver) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

var _ = context.Background
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/streamengine/ -run 'Driver|LiveSession' -v`
Expected: FAIL — `undefined: Driver`, `undefined: claudeDriver`, `undefined: DefaultDrivers`.

- [ ] **Step 3: Create the seam**

```go
// internal/streamengine/driver.go
package streamengine

import (
	"context"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/agui"
)

// LiveSession é uma sessão integrada viva, agnóstica de provider. O Manager fala
// só com esta interface; cada provider (Claude, OpenCode, …) fornece sua impl.
type LiveSession interface {
	Snapshot() agui.Snapshot
	SendPrompt(text string) error
	Respond(allow bool) error
	Close()
}

// Driver spawna uma LiveSession para um provider. onChange é chamado a cada
// transição de estado; persist grava cada linha do histórico (pode ser nil).
type Driver interface {
	Start(ctx context.Context, sessionID, cwd string, o Opts,
		onChange func(string), persist func(role, text string)) (LiveSession, error)
}

// claudeDriver é a impl de referência: o motor stream-json nativo do Claude Code.
type claudeDriver struct{}

func (claudeDriver) Start(ctx context.Context, sessionID, cwd string, o Opts,
	onChange func(string), persist func(role, text string)) (LiveSession, error) {
	return Start(ctx, sessionID, cwd, o, onChange, persist)
}

// DefaultDrivers é o registro de providers suportados no modo Integrado.
// agy/antigravity NÃO aparece aqui de propósito: não tem protocolo stream.
func DefaultDrivers() map[string]Driver {
	return map[string]Driver{
		"claude-code": claudeDriver{},
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/streamengine/ -run 'Driver|LiveSession' -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/streamengine/driver.go internal/streamengine/driver_test.go
git commit -m "feat(engine): extract provider-agnostic Driver/LiveSession seam"
```

---

## Task 2: Route `Manager.Start` by provider

**Files:**
- Modify: `internal/streamengine/manager.go:11-41`
- Test: `internal/streamengine/driver_test.go` (append)

**Interfaces:**
- Consumes: `Driver`, `LiveSession`, `DefaultDrivers()` from Task 1.
- Produces:
  - `Manager.sessions` becomes `map[string]LiveSession`.
  - `func NewManager(onChange func(string), persist func(sessionID, role, text string)) *Manager` — unchanged signature; internally seeds `drivers: DefaultDrivers()`.
  - `func (m *Manager) Start(ctx context.Context, provider, sessionID, cwd string, o Opts) error` — **new `provider` param (second position)**.
  - `var ErrUnknownProvider = errUnknownProvider{}` with `Error() string`.

- [ ] **Step 1: Write the failing test**

```go
// append to internal/streamengine/driver_test.go
func TestManagerStartUnknownProvider(t *testing.T) {
	m := NewManager(nil, nil)
	err := m.Start(context.Background(), "antigravity", "s1", t.TempDir(), Opts{})
	if err != ErrUnknownProvider {
		t.Fatalf("quer ErrUnknownProvider p/ provider sem driver, veio: %v", err)
	}
	if m.Has("s1") {
		t.Fatal("não deveria registrar sessão para provider desconhecido")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/streamengine/ -run 'TestManagerStartUnknownProvider' -v`
Expected: FAIL — too many arguments to `m.Start` / `undefined: ErrUnknownProvider`.

- [ ] **Step 3: Update the Manager**

Replace the `Manager` struct, `NewManager`, and `Start` in `internal/streamengine/manager.go` with:

```go
// Manager mantém as sessões integradas vivas, indexadas por id, e o registro de
// drivers por provider.
type Manager struct {
	mu       sync.Mutex
	sessions map[string]LiveSession
	drivers  map[string]Driver
	onChange func(sessionID string)
	persist  func(sessionID, role, text string)
}

// NewManager cria o gerenciador com os drivers padrão (DefaultDrivers).
func NewManager(onChange func(string), persist func(sessionID, role, text string)) *Manager {
	return &Manager{
		sessions: map[string]LiveSession{},
		drivers:  DefaultDrivers(),
		onChange: onChange,
		persist:  persist,
	}
}

// Start spawna e registra uma sessão do provider dado no cwd, com as opções.
func (m *Manager) Start(ctx context.Context, provider, sessionID, cwd string, o Opts) error {
	m.mu.Lock()
	drv := m.drivers[provider]
	m.mu.Unlock()
	if drv == nil {
		return ErrUnknownProvider
	}
	var persist func(role, text string)
	if m.persist != nil {
		persist = func(role, text string) { m.persist(sessionID, role, text) }
	}
	s, err := drv.Start(ctx, sessionID, cwd, o, m.onChange, persist)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.sessions[sessionID] = s
	m.mu.Unlock()
	return nil
}

// ErrUnknownProvider indica que o provider não tem driver no modo Integrado.
var ErrUnknownProvider = errUnknownProvider{}

type errUnknownProvider struct{}

func (errUnknownProvider) Error() string { return "provider não suportado no modo integrado" }
```

Also change the field type referenced in `get()`/`Snapshot()` — `get` already returns `*Session`; update it to return `LiveSession`:

```go
func (m *Manager) get(sessionID string) LiveSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[sessionID]
}
```

(`get` returning `LiveSession` keeps `Snapshot/SendPrompt/Respond/Close` callers working unchanged — they only use interface methods. The `nil`-check `if s == nil` still works because an unset map value is a nil interface.)

- [ ] **Step 4: Run test to verify it fails to compile at the call site**

Run: `go build ./...`
Expected: FAIL — `cmd/worrel/main.go` and `internal/httpapi/engine_session.go` call `Start` with the old arity. This is expected; Task 3 fixes the handler and Task 7 fixes any remaining caller. To keep this task green in isolation, update `cmd/worrel/main.go` engine wiring now: there is no `Start` call there (only `NewManager`), so `go build ./...` should only fail in `engine_session.go`. Proceed to fix it in Step 5.

- [ ] **Step 5: Update the handler call site minimally**

In `internal/httpapi/engine_session.go`, change the single call (full provider decode comes in Task 3):

```go
// temporário: provider fixo até a Task 3 ligar o request.
if err := s.deps.Engine.Start(context.Background(), "claude-code", sess.ID, cwd, opts); err != nil {
```

- [ ] **Step 6: Run build + tests**

Run: `go build ./... && go test ./internal/streamengine/ -v`
Expected: build OK; all streamengine tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/streamengine/manager.go internal/streamengine/driver_test.go internal/httpapi/engine_session.go
git commit -m "feat(engine): route Manager.Start by provider id"
```

---

## Task 3: Handler decodes provider and blocks antigravity

**Files:**
- Modify: `internal/httpapi/engine_session.go:28-39,83`
- Test: `internal/httpapi/engine_session_test.go` (**new**)

**Interfaces:**
- Consumes: `streamengine.ErrUnknownProvider`.
- Produces: request body now includes `provider string` (json `"provider"`); empty/`"claude-code"` keep current behavior; `"antigravity"` → HTTP 400.

- [ ] **Step 1: Write the failing test**

```go
// internal/httpapi/engine_session_test.go
package httpapi

import "testing"

// normalizeProvider é a regra de roteamento testável isolada do handler.
func TestNormalizeProvider(t *testing.T) {
	cases := map[string]struct {
		out string
		ok  bool
	}{
		"":            {"claude-code", true}, // default
		"claude-code": {"claude-code", true},
		"opencode":    {"opencode", true},
		"antigravity": {"", false}, // bloqueado: sem protocolo stream
	}
	for in, want := range cases {
		got, ok := normalizeProvider(in)
		if got != want.out || ok != want.ok {
			t.Errorf("normalizeProvider(%q) = (%q,%v), quer (%q,%v)", in, got, ok, want.out, want.ok)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/httpapi/ -run TestNormalizeProvider -v`
Expected: FAIL — `undefined: normalizeProvider`.

- [ ] **Step 3: Implement `normalizeProvider` and wire the handler**

Add to `internal/httpapi/engine_session.go`:

```go
// normalizeProvider valida o provider do modo Integrado. "" vira o default
// (claude-code). antigravity é bloqueado: o binário `agy` não tem protocolo
// stream (sem --output-format), então não há como dirigir a sessão.
func normalizeProvider(p string) (string, bool) {
	if p == "" {
		return "claude-code", true
	}
	if p == "antigravity" {
		return "", false
	}
	return p, true
}
```

Update the decode struct and the `Start` call:

```go
	in, _ := decode[struct {
		ProjectID string `json:"project_id"`
		Provider  string `json:"provider"` // "" = claude-code; antigravity bloqueado
		Mode      string `json:"mode"`
		Memory    string `json:"memory"`
	}](r)

	provider, ok := normalizeProvider(in.Provider)
	if !ok {
		writeErr(w, 400, "provider não suportado no modo integrado (use o modo Clássico)")
		return
	}
```

And the spawn (replace the temporary line from Task 2):

```go
	if err := s.deps.Engine.Start(context.Background(), provider, sess.ID, cwd, opts); err != nil {
		_ = s.deps.Store.EndSession(sess.ID)
		writeErr(w, 500, err.Error())
		return
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/httpapi/ -run TestNormalizeProvider -v && go build ./...`
Expected: PASS; build OK.

- [ ] **Step 5: Commit**

```bash
git add internal/httpapi/engine_session.go internal/httpapi/engine_session_test.go
git commit -m "feat(engine): handler decodes provider, blocks antigravity from Integrated"
```

---

## Task 4: ACP spike — capture the tool-permission exchange

**Files:**
- Create: `docs/superpowers/spikes/2026-06-23-opencode-acp-permission.md` (captured transcript + notes)

This is a **spike** (throwaway investigation), not production code. We have verified `initialize`, `session/new`, `session/prompt`, `agent_message_chunk`, and `stopReason`. We have NOT yet captured the live shapes of `tool_call`, `tool_call_update`, and the server→client `session/request_permission` request. Capture them before Task 6 so the permission handler maps real fields, not guessed ones.

- [ ] **Step 1: Run a prompt that forces a tool + permission**

Run this probe (requires `opencode` on PATH, authenticated):

```bash
cat > /tmp/acp_perm.py <<'PY'
import subprocess, json, threading, time
p = subprocess.Popen(["opencode","acp","--cwd","/tmp/acptest"],
    stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True, bufsize=1)
def send(o): p.stdin.write(json.dumps(o)+"\n"); p.stdin.flush()
def reader():
    for line in p.stdout:
        line=line.strip()
        if not line: continue
        try: o=json.loads(line)
        except: continue
        if o.get("id")==2 and "result" in o:
            sid=o["result"]["sessionId"]
            send({"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":sid,
              "prompt":[{"type":"text","text":"Run the shell command `echo hi` using your bash tool."}]}})
        elif o.get("method")=="session/update":
            u=o["params"]["update"]
            if u.get("sessionUpdate") in ("tool_call","tool_call_update"):
                print("TOOLCALL::", json.dumps(u)[:800])
        elif o.get("method")=="session/request_permission":
            print("PERMREQ::", json.dumps(o)[:800])
            # respond: select the first allow-ish option
            opts=o["params"].get("options",[])
            pick=next((x["optionId"] for x in opts if "allow" in x.get("optionId","").lower() or x.get("kind")=="allow_once"), opts[0]["optionId"] if opts else None)
            send({"jsonrpc":"2.0","id":o["id"],"result":{"outcome":{"outcome":"selected","optionId":pick}}})
threading.Thread(target=reader,daemon=True).start()
send({"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1,"clientCapabilities":{"fs":{"readTextFile":False,"writeTextFile":False}}}})
time.sleep(1.2)
send({"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":"/tmp/acptest","mcpServers":[]}})
time.sleep(25)
PY
python3 /tmp/acp_perm.py 2>&1 | head -60
```

Expected: at least one `PERMREQ::` line showing `params.options[].optionId` and `params.toolCall`, and `TOOLCALL::` lines showing `toolCallId`, `title`/`rawInput`, and `status`.

- [ ] **Step 2: Record the captured shapes**

Write `docs/superpowers/spikes/2026-06-23-opencode-acp-permission.md` with the exact JSON of one `tool_call`, one `tool_call_update`, and one `session/request_permission`, plus the field names Task 6 will read: `params.options[].optionId`, `params.options[].kind` (e.g. `allow_once`/`reject_once`), `params.toolCall.title`. If `opencode` auto-approves in headless ACP and never emits `request_permission`, record that fact — then Task 6's permission handler is a no-op for opencode and the `Respond` method just returns nil (document why).

- [ ] **Step 3: Commit the spike notes**

```bash
git add docs/superpowers/spikes/2026-06-23-opencode-acp-permission.md
git commit -m "docs(spike): capture opencode ACP tool_call + request_permission shapes"
```

---

## Task 5: ACP client + `opencodeDriver` happy path (text turn)

**Files:**
- Create: `internal/streamengine/opencode_acp.go`
- Test: `internal/streamengine/opencode_acp_test.go`

**Interfaces:**
- Consumes: `LiveSession`, `Driver`, `Opts`, `agui.*`.
- Produces:
  - `type opencodeDriver struct{}` implementing `Driver`.
  - `type acpSession struct { ... }` implementing `LiveSession`.
  - `func (s *acpSession) handleUpdate(update map[string]any)` — maps one ACP `session/update.update` onto state. **Task 6 extends this for tool_call/permission.**
  - `func (s *acpSession) Snapshot() agui.Snapshot`, `SendPrompt`, `Respond`, `Close`.
- The ACP wire layer (`session/prompt` request, reading the connection) is internal; tests drive `handleUpdate` directly with canned maps, so no real process is needed.

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/streamengine/ -run ACP -v`
Expected: FAIL — `undefined: acpSession`, `opencodeDriver`.

- [ ] **Step 3: Implement the ACP client + driver**

```go
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
	acpSID    string                       // sessionId devolvido por session/new
	state     agui.State
	message   string                       // texto acumulado do turno atual
	chunks    map[string]string            // messageId → texto acumulado
	toolCalls []agui.ToolCall
	history   []agui.HistoryLine
	interrupt *agui.Interrupt
	permID    int                          // id da request_permission pendente (0 = nenhuma)
	permOpts  []acpPermOption              // opções da permissão pendente
	pending   map[int]chan map[string]any  // id → canal de resposta (requests do cliente)
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
	_ = s.stdinW.Close()
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
	resp := <-ch
	if e, ok := resp["error"].(map[string]any); ok {
		return nil, fmt.Errorf("acp %s: %v", method, e["message"])
	}
	out, _ := resp["result"].(map[string]any)
	return out, nil
}

func (s *acpSession) readLoop(r *bufio.Reader) {
	dec := json.NewDecoder(r)
	for {
		var msg map[string]any
		if err := dec.Decode(&msg); err != nil {
			s.mu.Lock()
			s.state = agui.StateEnded
			s.mu.Unlock()
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
		s.appendChunk(u, true)
	case "agent_thought_chunk":
		// reasoning não vira mensagem (espelha o Claude, que ignora thinking).
	case "tool_call", "tool_call_update":
		s.handleToolCall(u) // Task 6
	}
}

func (s *acpSession) appendChunk(u map[string]any, message bool) {
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
func (s *acpSession) handleToolCall(u map[string]any)        {}
func (s *acpSession) handlePermissionRequest(msg map[string]any) {}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/streamengine/ -run 'ACP|OpencodeDriver' -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Register the driver**

In `internal/streamengine/driver.go`, add opencode to `DefaultDrivers`:

```go
func DefaultDrivers() map[string]Driver {
	return map[string]Driver{
		"claude-code": claudeDriver{},
		"opencode":    opencodeDriver{},
	}
}
```

- [ ] **Step 6: Run build + tests**

Run: `go build ./... && go test ./internal/streamengine/ -v`
Expected: build OK; all PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/streamengine/opencode_acp.go internal/streamengine/opencode_acp_test.go internal/streamengine/driver.go
git commit -m "feat(engine): OpenCode ACP driver — text-turn happy path"
```

---

## Task 6: ACP tool calls + permission, persistence + history

**Files:**
- Modify: `internal/streamengine/opencode_acp.go` (`handleToolCall`, `handlePermissionRequest`, `Respond`, `appendChunk` history persist)
- Test: `internal/streamengine/opencode_acp_test.go` (append)

**Interfaces:**
- Consumes: spike shapes from Task 4 (`docs/superpowers/spikes/2026-06-23-opencode-acp-permission.md`) — use the **actual** captured field names for `tool_call` (`toolCallId`/`title`/`rawInput`/`status`) and `session/request_permission` (`params.toolCall`, `params.options[].optionId`, `params.options[].kind`).
- Produces: completed `acpSession.Respond(allow bool) error`, `handleToolCall`, `handlePermissionRequest`; `agent_message_chunk` now also appends an `ai` history line at end-of-turn and persists it.

> **Implementer note:** The exact JSON field names below (`toolCallId`, `title`, `options`, `optionId`, `kind`) are the ACP spec defaults. **Reconcile them against the Task 4 spike capture before coding** — if opencode v1.17.9 names a field differently, use the captured name and update these tests to match.

- [ ] **Step 1: Write the failing tests**

```go
// append to internal/streamengine/opencode_acp_test.go
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
```

Add the test hook at the bottom of the test file:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/streamengine/ -run 'ACPToolCall|ACPPermission|ACPRespond' -v`
Expected: FAIL — `s.writeFn undefined`, interrupt not raised, ToolCalls empty.

- [ ] **Step 3: Implement tool calls, permission, and Respond**

Add a `writeFn` indirection field to `acpSession` (so `write` is testable) and replace the stubs:

```go
// in the acpSession struct, add:
//   writeFn func(any) error  // nil = usa o encoder real

func (s *acpSession) write(v any) error {
	if s.writeFn != nil {
		return s.writeFn(v)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enc.Encode(v)
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

// Respond responde a session/request_permission selecionando a opção allow/reject.
func (s *acpSession) Respond(allow bool) error {
	s.mu.Lock()
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
	if id == 0 {
		return fmt.Errorf("nenhuma permissão pendente")
	}
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
```

Also persist the assistant text + raise interrupt requirement in `handlePermissionRequest` is covered. Update `newTestACP()` if needed so `writeFn` defaults to nil (no change required — zero value is nil).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/streamengine/ -run 'ACP|Opencode' -v`
Expected: PASS.

- [ ] **Step 5: Persist assistant message at end of turn**

Update `onPromptResult` to flush the accumulated message into history + persist:

```go
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
```

Add a test:

```go
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
```

- [ ] **Step 6: Run all streamengine tests**

Run: `go test ./internal/streamengine/ -v`
Expected: PASS (all).

- [ ] **Step 7: Commit**

```bash
git add internal/streamengine/opencode_acp.go internal/streamengine/opencode_acp_test.go
git commit -m "feat(engine): ACP tool calls, permission round-trip, message persistence"
```

---

## Task 7: Live integration test (real opencode), gated

**Files:**
- Create: `internal/streamengine/opencode_live_test.go`

**Interfaces:**
- Consumes: `opencodeDriver{}`, `Opts{}`.
- Produces: a `//go:build integration`-gated test proving a real `opencode acp` round-trip.

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

func TestOpencodeACPLive(t *testing.T) {
	if _, err := exec.LookPath("opencode"); err != nil {
		t.Skip("opencode não instalado")
	}
	done := make(chan struct{}, 8)
	sess, err := opencodeDriver{}.Start(context.Background(), "live1", t.TempDir(),
		Opts{}, func(string) { select { case done <- struct{}{}: default {} } }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if err := sess.SendPrompt("responda apenas a palavra ok"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(60 * time.Second)
	for {
		select {
		case <-done:
			if sess.Snapshot().State == agui.StateAwaiting && sess.Snapshot().Message != "" {
				return // sucesso
			}
		case <-deadline:
			t.Fatalf("timeout; último snapshot: %+v", sess.Snapshot())
		}
	}
}
```

- [ ] **Step 2: Run it for real**

Run: `go test -tags integration ./internal/streamengine/ -run TestOpencodeACPLive -v -timeout 90s`
Expected: PASS — a real opencode ACP session returns text and ends the turn. (If it fails, the captured ACP shapes in Tasks 5–6 are wrong; fix the field mapping, not the test.)

- [ ] **Step 3: Commit**

```bash
git add internal/streamengine/opencode_live_test.go
git commit -m "test(engine): gated live opencode ACP round-trip"
```

---

## Task 8: Wire the frontend — pass provider, block agy from Integrated

**Files:**
- Modify: `web/src/api.ts:543-550` (`createEngineSession` signature + body)
- Modify: `web/src/components/NewSessionWizard.tsx:117-126` (pass `adapterId`); `:269-286` (disable Integrated for agy)
- Modify: `web/src/i18n.ts` (PT `:605-610`, EN `:58-63`) — add `integratedUnavailable` copy

**Interfaces:**
- Consumes: backend `POST /api/sessions/engine` now accepting `provider`.
- Produces: `createEngineSession(projectId?, mode?, memory?, provider?)`.

- [ ] **Step 1: Update the API client**

```ts
// web/src/api.ts
export function createEngineSession(projectId?: string, mode?: PermissionMode, memory?: MemoryMode, provider?: string): Promise<Session> {
  return req('/sessions/engine', {
    method: 'POST',
    body: JSON.stringify({ project_id: projectId ?? '', mode: mode ?? 'auto', memory: memory ?? 'inicio', provider: provider ?? '' }),
  });
}
```

- [ ] **Step 2: Pass the selected provider from the wizard**

In `web/src/components/NewSessionWizard.tsx`, the engine branch:

```ts
      // Sessão de motor (stream-json): passa o provider selecionado. O backend
      // valida (antigravity é bloqueado — sem protocolo stream).
      const sess = await createEngineSession(projectId ?? undefined, permMode, mode, adapterId);
```

- [ ] **Step 3: Disable Integrated for agy and add the hint copy**

Add to `web/src/i18n.ts` (PT block near `engineModeDesc`):

```ts
              integratedUnavailable: 'Integrado indisponível para este provider — use o Clássico.',
```

and the EN block:

```ts
              integratedUnavailable: 'Integrated unavailable for this provider — use Classic.',
```

In `NewSessionWizard.tsx`, force `classic` when the provider is antigravity and disable the Integrated button. Just above the mode buttons (`<div className="nsw-field">` at line ~270), add:

```tsx
              {adapterId === 'antigravity' && !classic && (
                // agy não tem protocolo stream → Integrado impossível; força Clássico.
                <>{(() => { setClassic(true); return null; })()}</>
              )}
```

and in the mode button, disable the Integrated (`isClassic === false`) option for agy:

```tsx
                    <button key={String(isClassic)} className={`nsw-mode${classic === isClassic ? ' on' : ''}`}
                      disabled={!isClassic && adapterId === 'antigravity'}
                      title={!isClassic && adapterId === 'antigravity' ? t('home.wizard.integratedUnavailable') : undefined}
                      onClick={() => setClassic(isClassic)} aria-pressed={classic === isClassic}>
```

> **Note:** Calling `setClassic` during render (the IIFE above) is a quick guard but triggers a re-render warning. Preferred: a `useEffect(() => { if (adapterId === 'antigravity') setClassic(true); }, [adapterId])`. Use the effect form; place it next to the other `useEffect`s (around line 82).

Replace the IIFE approach with the effect:

```tsx
  useEffect(() => {
    if (adapterId === 'antigravity') setClassic(true);
  }, [adapterId]);
```

- [ ] **Step 4: Typecheck the web build**

Run: `cd web && npm run build`
Expected: EXIT 0, no TypeScript errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/api.ts web/src/components/NewSessionWizard.tsx web/src/i18n.ts
git commit -m "feat(web): Integrated passes provider; block agy (no stream protocol)"
```

---

## Task 9: Full build + regression sweep

**Files:** none (verification only)

- [ ] **Step 1: Backend build + unit tests**

Run: `go build ./... && go test ./...`
Expected: all PASS (integration-tagged live tests are skipped without `-tags integration`).

- [ ] **Step 2: Backend live tests (opencode + claude), opt-in**

Run: `go test -tags integration ./internal/streamengine/ -v -timeout 120s`
Expected: PASS for `TestOpencodeACPLive` (and existing `TestEngineLive` for claude).

- [ ] **Step 3: Web build**

Run: `cd web && npm run build`
Expected: EXIT 0.

- [ ] **Step 4: Manual smoke (documented, not automated)**

Start the app, open New Session:
- Provider **OpenCode** + **Integrado** → Home miniature fills with streamed text in real time; the ordering (`markUsed('provider','opencode')`) now reflects reality and OpenCode rises to the top next time. ✅ (this is the original ordering bug, fixed by making Integrated actually honor the provider.)
- Provider **Antigravity** → Integrated button disabled with the hint; only Clássico available.
- Provider **Claude** + Integrado → unchanged behavior.

- [ ] **Step 5: Commit (if any doc tweaks)**

```bash
git add -A && git commit -m "chore(engine): multi-provider Integrated — build + regression sweep"
```

---

## Self-Review Notes

- **Spec coverage:** interface seam (T1) · provider routing (T2) · handler+agy block backend (T3) · ACP shapes spike (T4) · ACP happy path (T5) · tool/permission/persist (T6) · live test (T7) · frontend wiring + agy block UI (T8) · regression (T9). The original ordering bug is resolved structurally by T2+T8 (Integrated honors the provider, so `markUsed('provider', adapterId)` becomes truthful) — no special-case patch needed.
- **agy:** never added to `DefaultDrivers`; blocked at the handler (`normalizeProvider`) and in the UI (T8). Hard constraint honored.
- **Claude unchanged:** `claudeDriver` delegates to the existing `Start`; no claude code path was edited.
- **Codex:** intentionally out of scope for this plan (separate follow-up: a `codexDriver` over `codex app-server`/`exec-server`). The seam (T1–T2) makes it a drop-in addition.
- **Open risk:** ACP field names in T6 are spec-default; T4 spike + T7 live test are the guardrails that catch any mismatch against opencode v1.17.9.
</content>
</invoke>
