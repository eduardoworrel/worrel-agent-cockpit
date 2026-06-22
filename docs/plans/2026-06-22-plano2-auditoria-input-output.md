# Plano 2 — Auditoria input/output de IA Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Toda chamada de IA dos motores de borda da Home (resumo de progresso e interpretação) passa a gravar, de forma inegociável, o **prompt enviado (input)** e a **resposta crua (output)** na tabela `engine_log`, ancorados na sessão de origem; a aba "Atividade" exibe esses campos.

**Architecture:** Estende a tabela `engine_log` (já o registro de explicabilidade do projeto) com duas colunas `input`/`output` via o mecanismo idempotente `migrateAddColumns`. `EngineLogEntry`, `LogEngineRun` e `ListEngineLog` carregam os campos novos. Um helper `LogAICall` grava uma linha de auditoria a partir de uma chamada de IA de borda. Os pontos de borda em `interaction_summary.go` (`attachProgress`, `attachEngineSummary`) e `interaction_interpret.go` (`attachInterpretation`) chamam `LogAICall` após cada `RunHeadless`. O frontend mostra input/output expansível na aba "Atividade".

**Tech Stack:** Go 1.x (modernc.org/sqlite), testes `go test`. Frontend React/TS (sem novo runner; só renderização).

## Global Constraints

- Comentários e copy em **português** (padrão do repo).
- Migração de schema: **append-only** na lista de `migrateAddColumns` (`internal/store/store.go`), idempotente; nunca editar entradas existentes.
- Auditoria **inegociável**: não existe flag que desligue a gravação de input/output.
- Retenção: o registro vive e morre **exatamente como o `engine_log` vive hoje** — não introduzir poda nova.
- `engine_log` existente continua válido: linhas antigas têm `input`/`output` vazios (default `''`).

## Escopo / Fora de escopo

- **Neste plano:** infra de auditoria + captura de I/O cru dos motores de borda **resumo (summary)** e **interpretação (interpret)**.
- **Fora deste plano (Plano 2b, deferido):** captura de prompt/resposta crus dos motores de destilação **memory / skill / friction**, cujo LLM é chamado dentro de helpers internos (`NewRouter(...).Route(...)` etc.) e exige threading do I/O para fora desses helpers. Esses motores **já gravam** hoje uma linha `engine_log` por execução (metadados) via `Registry.Run`; apenas o I/O cru fica para o 2b.

---

### Task 1: Migração — colunas `input` e `output` em `engine_log`

**Files:**
- Modify: `internal/store/store.go:60-99` (lista `wanted` em `migrateAddColumns`)
- Test: `internal/store/engine_log_test.go` (criar)

**Interfaces:**
- Produces: tabela `engine_log` com colunas `input TEXT NOT NULL DEFAULT ''` e `output TEXT NOT NULL DEFAULT ''`.

- [ ] **Step 1: Escrever teste que falha (colunas existem após Open)**

Criar `internal/store/engine_log_test.go`:

```go
package store

import "testing"

func TestEngineLogHasInputOutputColumns(t *testing.T) {
	st, err := Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	for _, col := range []string{"input", "output"} {
		var n int
		if err := st.DB().QueryRow(
			`SELECT count(*) FROM pragma_table_info('engine_log') WHERE name=?`, col,
		).Scan(&n); err != nil {
			t.Fatalf("pragma: %v", err)
		}
		if n != 1 {
			t.Fatalf("coluna %q ausente em engine_log", col)
		}
	}
}
```

- [ ] **Step 2: Rodar o teste e ver falhar**

Run: `go test ./internal/store/ -run TestEngineLogHasInputOutputColumns -v`
Expected: FAIL — `coluna "input" ausente em engine_log`.

- [ ] **Step 3: Adicionar as duas entradas em `migrateAddColumns`**

Em `internal/store/store.go`, no final da slice `wanted` (após a entrada `{"agents", "active_generation", ...}`, antes do `}` que fecha a slice), adicionar:

```go
		// auditoria de IA: prompt enviado e resposta crua do modelo, por
		// execução de motor. Vazio quando não houve chamada de IA (heurística).
		{"engine_log", "input",
			`ALTER TABLE engine_log ADD COLUMN input TEXT NOT NULL DEFAULT ''`},
		{"engine_log", "output",
			`ALTER TABLE engine_log ADD COLUMN output TEXT NOT NULL DEFAULT ''`},
```

- [ ] **Step 4: Rodar o teste e ver passar**

Run: `go test ./internal/store/ -run TestEngineLogHasInputOutputColumns -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/engine_log_test.go
git commit -m "feat(store): colunas input/output em engine_log para auditoria de IA"
```

---

### Task 2: `EngineLogEntry` + `LogEngineRun` + `ListEngineLog` carregam input/output

**Files:**
- Modify: `internal/store/engine_log.go` (todo o arquivo)
- Modify: `internal/store/engine_log_test.go` (adicionar teste de round-trip)

**Interfaces:**
- Consumes: colunas da Task 1.
- Produces:
  - `EngineLogEntry` com campos `Input string` e `Output string` (JSON `input`/`output`).
  - `LogEngineRun(e *EngineLogEntry) error` grava também `e.Input`, `e.Output`.
  - `ListEngineLog(limit int) ([]*EngineLogEntry, error)` devolve `Input`/`Output`.

- [ ] **Step 1: Escrever teste de round-trip (falha)**

Adicionar a `internal/store/engine_log_test.go`:

```go
func TestLogEngineRunPersistsInputOutput(t *testing.T) {
	st, err := Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	if err := st.LogEngineRun(&EngineLogEntry{
		EngineID: "summary", SessionID: "s1",
		Trigger: "realtime", Suggestions: 0, Detail: "",
		Input: "PROMPT-X", Output: "RESPOSTA-Y",
	}); err != nil {
		t.Fatalf("log: %v", err)
	}
	got, err := st.ListEngineLog(10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].Input != "PROMPT-X" || got[0].Output != "RESPOSTA-Y" {
		t.Fatalf("input/output não persistiram: %+v", got)
	}
}
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/store/ -run TestLogEngineRunPersistsInputOutput -v`
Expected: FAIL — campo `Input`/`Output` não existe em `EngineLogEntry` (erro de compilação).

- [ ] **Step 3: Reescrever `internal/store/engine_log.go`**

```go
package store

// engine_log registra cada execução de um motor para explicabilidade: quando
// rodou, em qual sessão/projeto, sob qual gatilho, quantas sugestões gerou e
// quais (títulos). Para chamadas de IA grava também o prompt (input) e a
// resposta crua do modelo (output). Alimenta a aba "Atividade" da config.

type EngineLogEntry struct {
	ID          int64  `json:"id"`
	EngineID    string `json:"engine_id"`
	ProjectID   string `json:"project_id"`
	SessionID   string `json:"session_id"`
	Trigger     string `json:"trigger"`
	Suggestions int    `json:"suggestions"`
	Detail      string `json:"detail"`
	Input       string `json:"input"`  // prompt enviado à IA ('' = sem IA)
	Output      string `json:"output"` // resposta crua do modelo ('' = sem IA)
	CreatedAt   int64  `json:"created_at"`
}

func (s *Store) LogEngineRun(e *EngineLogEntry) error {
	res, err := s.db.Exec(`INSERT INTO engine_log
		(engine_id, project_id, session_id, trigger, suggestions, detail, input, output, created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		e.EngineID, e.ProjectID, e.SessionID, e.Trigger, e.Suggestions, e.Detail,
		e.Input, e.Output, now())
	if err != nil {
		return err
	}
	e.ID, _ = res.LastInsertId()
	return nil
}

// ListEngineLog devolve as execuções mais recentes (até limit), das mais novas
// para as mais antigas.
func (s *Store) ListEngineLog(limit int) ([]*EngineLogEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT id, engine_id, COALESCE(project_id,''), COALESCE(session_id,''),
		trigger, suggestions, detail, input, output, created_at
		FROM engine_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*EngineLogEntry{}
	for rows.Next() {
		e := &EngineLogEntry{}
		if err := rows.Scan(&e.ID, &e.EngineID, &e.ProjectID, &e.SessionID,
			&e.Trigger, &e.Suggestions, &e.Detail, &e.Input, &e.Output, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Rodar e ver passar (e não quebrar o resto do store)**

Run: `go test ./internal/store/ -v`
Expected: PASS — incluindo os testes existentes do store (o `INSERT` antigo via `LogEngineRun` continua válido porque o struct preenche Input/Output como `""` por default).

- [ ] **Step 5: Commit**

```bash
git add internal/store/engine_log.go internal/store/engine_log_test.go
git commit -m "feat(store): EngineLogEntry carrega input/output da IA"
```

---

### Task 3: Logar I/O do resumo de progresso (`attachProgress` + `attachEngineSummary`)

**Files:**
- Modify: `internal/httpapi/interaction_summary.go:96-120` (goroutine de `attachEngineSummary`) e `:138-160` (goroutine de `attachProgress`)
- Test: `internal/httpapi/interaction_summary_test.go` (adicionar)

**Interfaces:**
- Consumes: `s.deps.Store.LogEngineRun` (Task 2); `s.deps.Summarizer.RunHeadless`.
- Produces: após cada `RunHeadless` bem-sucedido do resumo, uma linha `engine_log` com `EngineID:"summary"`, `SessionID:id`, `Trigger:"realtime"`, `Input:prompt`, `Output:out`.

- [ ] **Step 1: Escrever teste (falha) — uma chamada de resumo grava auditoria**

Adicionar a `internal/httpapi/interaction_summary_test.go` (o arquivo já define `fakeHeadless` e um helper de server; reutilizar). Se o helper de server existente não injeta `Store`, criar um server local com store real:

```go
func TestAttachProgress_LogsAudit(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	srv := &Server{deps: Deps{
		Bus:        bus.New(),
		Store:      st,
		Summarizer: &fakeHeadless{out: `{"title":"Foo","lines":["fez X"]}`},
	}, progress: newProgressCache()}

	snap := &agui.Snapshot{SessionID: "s1", State: agui.StateBusy}
	events := []*store.TranscriptEvent{
		{Role: "user", Kind: "text", Content: "oi"},
		{Role: "assistant", Kind: "text", Content: "fazendo X"},
		{Role: "assistant", Kind: "text", Content: "fazendo Y"},
	}
	srv.attachProgress(snap, events)

	// a geração é assíncrona; espera curta determinística por polling do log.
	var got []*store.EngineLogEntry
	for i := 0; i < 100; i++ {
		got, _ = st.ListEngineLog(10)
		if len(got) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(got) != 1 {
		t.Fatalf("esperava 1 linha de auditoria, veio %d", len(got))
	}
	if got[0].EngineID != "summary" || got[0].SessionID != "s1" ||
		got[0].Input == "" || got[0].Output == "" {
		t.Fatalf("auditoria incompleta: %+v", got[0])
	}
}
```

Garantir os imports necessários no topo do arquivo de teste: `"time"`, `"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"`, `"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"`, `"github.com/eduardoworrel/worrel-agent-cockpit/internal/agui"`. Conferir/ajustar o struct literal de `fakeHeadless` para o campo que ele já usa para devolver a saída (no arquivo atual `fakeHeadless` devolve um valor fixo; adaptar para aceitar `out string` se ainda não aceitar — manter compatível com os testes existentes).

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/httpapi/ -run TestAttachProgress_LogsAudit -v`
Expected: FAIL — nenhuma linha de auditoria gravada (`esperava 1 linha, veio 0`).

- [ ] **Step 3: Logar I/O na goroutine de `attachProgress`**

Em `internal/httpapi/interaction_summary.go`, dentro da goroutine de `attachProgress`, logo após `out, err := s.deps.Summarizer.RunHeadless(...)` retornar com `err == nil` e antes de `s.progress.store(...)`, inserir:

```go
		_ = s.deps.Store.LogEngineRun(&store.EngineLogEntry{
			EngineID: "summary", SessionID: id, Trigger: "realtime",
			Input: prompt, Output: out,
		})
```

(`prompt` já está no escopo da goroutine; `out` é a resposta crua; `id` é o session id capturado.)

- [ ] **Step 4: Logar I/O na goroutine de `attachEngineSummary`**

Em `internal/httpapi/interaction_summary.go`, dentro da goroutine de `attachEngineSummary`, logo após o `RunHeadless` retornar `err == nil` e antes de `s.titles.store(...)`, inserir o mesmo bloco:

```go
		_ = s.deps.Store.LogEngineRun(&store.EngineLogEntry{
			EngineID: "summary", SessionID: id, Trigger: "realtime",
			Input: prompt, Output: out,
		})
```

- [ ] **Step 5: Rodar e ver passar (e suíte do pacote)**

Run: `go test ./internal/httpapi/ -run TestAttachProgress_LogsAudit -v && go test ./internal/httpapi/`
Expected: PASS nos dois. (O teste existente `TestAttachProgress_NoSummarizerNoop` continua válido — sem Summarizer não há RunHeadless nem log.)

- [ ] **Step 6: Commit**

```bash
git add internal/httpapi/interaction_summary.go internal/httpapi/interaction_summary_test.go
git commit -m "feat(audit): grava input/output do resumo de progresso em engine_log"
```

---

### Task 4: Logar I/O da interpretação (`attachInterpretation`)

**Files:**
- Modify: `internal/httpapi/interaction_interpret.go:81-92` (goroutine após `RunHeadless`)
- Test: `internal/httpapi/interaction_interpret_test.go` (criar)

**Interfaces:**
- Consumes: `s.deps.Store.LogEngineRun`; `s.deps.Summarizer.RunHeadless`.
- Produces: após cada `RunHeadless` da interpretação, linha `engine_log` com `EngineID:"interpret"`, `SessionID:id`, `Trigger:"agent_self"`, `Input:prompt`, `Output:out`.

- [ ] **Step 1: Escrever teste (falha)**

Criar `internal/httpapi/interaction_interpret_test.go`:

```go
package httpapi

import (
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/agui"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func TestAttachInterpretation_LogsAudit(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	srv := &Server{deps: Deps{
		Bus:        bus.New(),
		Store:      st,
		Summarizer: &fakeHeadless{out: `{"kind":"text","prompt":"e agora?","options":[]}`},
	}, interpret: newInterpretCache()}

	snap := &agui.Snapshot{
		SessionID: "s1",
		State:     agui.StateAwaiting,
		Message:   "Quer que eu continue?",
		History:   []agui.HistoryLine{{Role: "assistant", Text: "Quer que eu continue?"}},
	}
	srv.attachInterpretation(snap)

	var got []*store.EngineLogEntry
	for i := 0; i < 100; i++ {
		got, _ = st.ListEngineLog(10)
		if len(got) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(got) != 1 || got[0].EngineID != "interpret" ||
		got[0].Input == "" || got[0].Output == "" {
		t.Fatalf("auditoria da interpretação incompleta: %+v", got)
	}
}
```

> Antes de implementar: ler `internal/httpapi/interaction_interpret.go:67-92` para confirmar a condição de guarda de `attachInterpretation` (estado, `Interrupt == nil`, `msg` não-vazio) e ajustar o `snap` do teste para satisfazê-la. Ajustar nomes de constantes de estado (`agui.StateAwaiting` etc.) ao que o pacote `agui` realmente exporta.

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/httpapi/ -run TestAttachInterpretation_LogsAudit -v`
Expected: FAIL — nenhuma auditoria gravada.

- [ ] **Step 3: Logar I/O na goroutine de `attachInterpretation`**

Em `internal/httpapi/interaction_interpret.go`, dentro da goroutine, logo após `out, err := s.deps.Summarizer.RunHeadless(...)` retornar `err == nil` e antes de `s.interpret.store(...)`, inserir:

```go
		_ = s.deps.Store.LogEngineRun(&store.EngineLogEntry{
			EngineID: "interpret", SessionID: id, Trigger: "agent_self",
			Input: prompt, Output: out,
		})
```

Garantir o import de `internal/store` no arquivo (se ainda não houver). `prompt`, `out`, `id` já estão no escopo da goroutine (confirmar nomes ao ler o arquivo).

- [ ] **Step 4: Rodar e ver passar (e suíte)**

Run: `go test ./internal/httpapi/ -run TestAttachInterpretation_LogsAudit -v && go test ./internal/httpapi/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/httpapi/interaction_interpret.go internal/httpapi/interaction_interpret_test.go
git commit -m "feat(audit): grava input/output da interpretação em engine_log"
```

---

### Task 5: Aba "Atividade" exibe input/output (expansível)

**Files:**
- Modify: `web/src/pages/Settings.tsx:9` (tipo `EngineLogEntry`)
- Modify: `web/src/pages/Settings.tsx:199-209` (render de cada linha de atividade)

**Interfaces:**
- Consumes: JSON de `GET /api/engines/activity` agora com `input`/`output` (Task 2).
- Produces: cada linha com IA mostra um `<details>` com o prompt e a resposta crua.

- [ ] **Step 1: Estender o tipo `EngineLogEntry` no Settings**

Em `web/src/pages/Settings.tsx`, linha 9, adicionar os campos `input` e `output` ao tipo:

```ts
type EngineLogEntry = { id: number; engine_id: string; trigger: string; suggestions: number; detail: string; input?: string; output?: string; created_at: number };
```

(Manter os campos já existentes exatamente como estão; apenas acrescentar `input?` e `output?`.)

- [ ] **Step 2: Renderizar input/output quando presentes**

Em `web/src/pages/Settings.tsx`, dentro do `activity.map(a => (...))`, logo após o bloco `{a.detail && <div ...>{a.detail}</div>}`, adicionar:

```tsx
              {(a.input || a.output) && (
                <details style={{ marginTop: '0.3rem', fontSize: '0.8rem' }}>
                  <summary style={{ cursor: 'pointer', color: 'var(--muted)' }}>
                    {t('settings.activityIO', 'Ver prompt e resposta da IA')}
                  </summary>
                  {a.input && (
                    <pre style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word', background: 'var(--bg-elev, #1a1a1a)', padding: '0.5rem', borderRadius: 4, marginTop: '0.3rem' }}>
                      <strong>input:</strong>{'\n'}{a.input}
                    </pre>
                  )}
                  {a.output && (
                    <pre style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word', background: 'var(--bg-elev, #1a1a1a)', padding: '0.5rem', borderRadius: 4, marginTop: '0.3rem' }}>
                      <strong>output:</strong>{'\n'}{a.output}
                    </pre>
                  )}
                </details>
              )}
```

- [ ] **Step 3: Atualizar o hint da aba para refletir a auditoria de I/O**

Em `web/src/pages/Settings.tsx:196`, trocar o texto-default do hint:

```tsx
            {t('settings.activityHint', 'Cada execução de motor, as sugestões que gerou e o prompt/resposta da IA quando houver (explicabilidade).')}
          </p>
```

- [ ] **Step 4: Type-check, lint e build**

Run: `cd web && npx tsc -b && npx eslint . && npm run build`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/Settings.tsx
git commit -m "feat(audit): aba Atividade mostra input/output da IA"
```
