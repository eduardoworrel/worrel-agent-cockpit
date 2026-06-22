# Plano 3 — Toggles de custo dos motores de IA da Home Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Permitir ligar/desligar os dois motores de IA da Home: **resumo de progresso** (`summary`) com toggle **global (default OFF) + override por-miniatura**, e **interpretação** (`interpret`) com toggle **só global (default ON)**. Quando o resumo está OFF, a miniatura mostra a **cauda congelada** do histórico cru (sem rolagem, mostrando o final), sem custo de IA.

**Architecture:** `summary`/`interpret` não são motores do Registry — são funções de borda em `internal/httpapi`. O toggle é config (`engine_config`, chave `__enabled`) resolvida na borda antes de chamar `RunHeadless`. Escopo por-sessão usa `scope_key="session:<id>"` (o `SetEngineConfig` já aceita scope arbitrário). Um helper `Store.EngineEnabled` resolve sessão ⊕ global ⊕ default. O frontend lê/escreve via endpoint dedicado e a `TerminalCard` ganha um switch; OFF → renderiza a cauda do `snapshot.history`.

**Tech Stack:** Go (modernc.org/sqlite), `go test`. React 19/TS, Vite (sem runner de teste de FE — verificação por `tsc -b`/`eslint`/`build`, padrão do repo).

## Global Constraints

- Comentários e copy em **português**.
- A escolha de **harness/modelo** desses dois motores de borda **NÃO** entra neste plano — fica no Plano 4 (onboarding/settings), que também monta a UI global definitiva. Aqui entregamos só os toggles on/off + o estado OFF da miniatura + um controle global mínimo em Settings.
- Auditoria (Plano 2) é inegociável: quando o motor roda (ON), continua gravando input/output — não mexer nisso.
- Defaults: `summary` = OFF; `interpret` = ON.
- Frontend sem dependências novas.

---

### Task 1: `Store.EngineEnabled` resolve sessão ⊕ global ⊕ default

**Files:**
- Create: `internal/store/engine_enabled.go`
- Test: `internal/store/engine_enabled_test.go`

**Interfaces:**
- Consumes: `SetEngineConfig(engineID, key, value, scopeKey string)` e `GetEngineConfig(engineID, scopeKey string)` (já existem).
- Produces: `func (s *Store) EngineEnabled(engineID, sessionID string, defaultOn bool) bool`. Resolve `__enabled`: se `sessionID != ""` e houver override em `scope_key="session:"+sessionID`, vence; senão o global (`scope_key=""`); senão `defaultOn`. Valor `"true"` → true; `"false"` → false.

- [ ] **Step 1: Escrever testes (falham)**

Criar `internal/store/engine_enabled_test.go`:

```go
package store

import "testing"

func TestEngineEnabledDefault(t *testing.T) {
	st, _ := Open(t.TempDir() + "/t.db")
	defer st.Close()
	if st.EngineEnabled("summary", "s1", false) != false {
		t.Fatal("sem config, deve cair no default false")
	}
	if st.EngineEnabled("interpret", "", true) != true {
		t.Fatal("sem config, deve cair no default true")
	}
}

func TestEngineEnabledGlobalOverride(t *testing.T) {
	st, _ := Open(t.TempDir() + "/t.db")
	defer st.Close()
	if err := st.SetEngineConfig("summary", "__enabled", "true", ""); err != nil {
		t.Fatal(err)
	}
	if st.EngineEnabled("summary", "s1", false) != true {
		t.Fatal("global true deve sobrepor o default false")
	}
}

func TestEngineEnabledSessionBeatsGlobal(t *testing.T) {
	st, _ := Open(t.TempDir() + "/t.db")
	defer st.Close()
	_ = st.SetEngineConfig("summary", "__enabled", "true", "")             // global ON
	_ = st.SetEngineConfig("summary", "__enabled", "false", "session:s1") // sessão OFF
	if st.EngineEnabled("summary", "s1", false) != false {
		t.Fatal("override de sessão deve vencer o global")
	}
	if st.EngineEnabled("summary", "s2", false) != true {
		t.Fatal("outra sessão sem override segue o global ON")
	}
}
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/store/ -run TestEngineEnabled -v`
Expected: FAIL — `st.EngineEnabled undefined` (erro de compilação).

- [ ] **Step 3: Implementar `engine_enabled.go`**

```go
package store

// EngineEnabled resolve o toggle __enabled de um motor de borda (summary,
// interpret) na ordem: override de sessão ("session:<id>") ⊕ global ("") ⊕
// defaultOn. sessionID vazio ignora a camada de sessão (toggles só-globais).
func (s *Store) EngineEnabled(engineID, sessionID string, defaultOn bool) bool {
	if sessionID != "" {
		if m, err := s.GetEngineConfig(engineID, "session:"+sessionID); err == nil {
			if v, ok := m["__enabled"]; ok {
				return v == "true"
			}
		}
	}
	if m, err := s.GetEngineConfig(engineID, ""); err == nil {
		if v, ok := m["__enabled"]; ok {
			return v == "true"
		}
	}
	return defaultOn
}
```

- [ ] **Step 4: Rodar e ver passar**

Run: `go test ./internal/store/ -run TestEngineEnabled -v`
Expected: PASS (3 testes).

- [ ] **Step 5: Commit**

```bash
git add internal/store/engine_enabled.go internal/store/engine_enabled_test.go
git commit -m "feat(store): EngineEnabled resolve toggle sessao/global/default"
```

---

### Task 2: Gatear o resumo de progresso (`attachProgress` + `attachEngineSummary`)

**Files:**
- Modify: `internal/httpapi/interaction_summary.go` (início de `attachProgress` e de `attachEngineSummary`)
- Test: `internal/httpapi/interaction_summary_test.go` (adicionar)

**Interfaces:**
- Consumes: `s.deps.Store.EngineEnabled("summary", id, false)`.
- Produces: quando OFF, a borda retorna sem chamar `RunHeadless` (sem custo, sem auditoria); `snap.Progress` fica como veio (vazio/última cache).

> Antes de editar: ler `interaction_summary.go` para confirmar onde cada função
> chama `RunHeadless` e onde inserir o early-return sem quebrar o set de
> `snap.Progress` a partir do cache (que pode permanecer para mostrar o último
> resumo já pago, se houver — aceitável). O gate deve impedir SOMENTE a nova
> chamada de IA.

- [ ] **Step 1: Escrever teste (falha) — OFF não chama RunHeadless**

O `fakeHeadless` do arquivo devolve `out` fixo. Estender o fake para contar chamadas (campo `calls int`), ou criar um fake local `countingHeadless`. Adicionar:

```go
func TestAttachProgress_DisabledSkipsLLM(t *testing.T) {
	st, _ := store.Open(t.TempDir() + "/t.db")
	defer st.Close()
	// summary OFF global (default já é false, mas fixamos explícito):
	_ = st.SetEngineConfig("summary", "__enabled", "false", "")

	sum := &fakeHeadless{out: `{"title":"X","lines":["y"]}`}
	srv := &Server{deps: Deps{Bus: bus.New(), Store: st, Summarizer: sum}, progress: newProgressCache()}

	snap := &agui.Snapshot{SessionID: "s1", State: agui.StateAwaiting}
	events := []*store.TranscriptEvent{
		{Role: "user", Kind: "text", Content: "oi"},
		{Role: "assistant", Kind: "text", Content: "a"},
		{Role: "assistant", Kind: "text", Content: "b"},
	}
	srv.attachProgress(snap, events)

	// dá tempo de uma eventual goroutine indevida rodar
	time.Sleep(50 * time.Millisecond)
	got, _ := st.ListEngineLog(10)
	if len(got) != 0 {
		t.Fatalf("summary OFF não deveria gerar auditoria/chamada; veio %d", len(got))
	}
}
```

(Se o `fakeHeadless` não permitir contagem, este teste usa a ausência de linha de auditoria — que só é gravada após `RunHeadless` bem-sucedido (Plano 2) — como prova de que a IA não rodou.)

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/httpapi/ -run TestAttachProgress_DisabledSkipsLLM -v`
Expected: FAIL — auditoria gravada (veio 1), pois a IA ainda roda.

- [ ] **Step 3: Inserir o gate em `attachProgress`**

Em `internal/httpapi/interaction_summary.go`, em `attachProgress`, logo após `snap.Progress = lines` e antes do `if s.deps.Summarizer == nil || snap.State == agui.StateEnded {`, inserir:

```go
	// toggle de custo: resumo desligado (global ou por-sessão) → não chama IA.
	if s.deps.Store != nil && !s.deps.Store.EngineEnabled("summary", snap.SessionID, false) {
		return
	}
```

- [ ] **Step 4: Inserir o gate em `attachEngineSummary`**

Em `attachEngineSummary`, logo após `if s.deps.Summarizer == nil { return }`, inserir:

```go
	if s.deps.Store != nil && !s.deps.Store.EngineEnabled("summary", snap.SessionID, false) {
		return
	}
```

- [ ] **Step 5: Rodar e ver passar (e suíte)**

Run: `go test ./internal/httpapi/ -run TestAttachProgress -v && go test ./internal/httpapi/`
Expected: PASS. O teste do Plano 2 (`TestAttachProgress_LogsAudit`) precisa agora ligar o summary: **se ele falhar por causa do gate**, editar aquele teste para `st.SetEngineConfig("summary","__enabled","true","")` antes de `attachProgress`. Aplicar essa edição e re-rodar até verde.

- [ ] **Step 6: Commit**

```bash
git add internal/httpapi/interaction_summary.go internal/httpapi/interaction_summary_test.go
git commit -m "feat(toggle): gate do resumo de progresso (summary) por EngineEnabled"
```

---

### Task 3: Gatear a interpretação (`attachInterpretation`, só global, default ON)

**Files:**
- Modify: `internal/httpapi/interaction_interpret.go` (início de `attachInterpretation`)
- Test: `internal/httpapi/interaction_interpret_test.go` (adicionar)

**Interfaces:**
- Consumes: `s.deps.Store.EngineEnabled("interpret", "", true)` (sessionID vazio = só global).

- [ ] **Step 1: Escrever teste (falha) — OFF global pula a IA**

```go
func TestAttachInterpretation_DisabledSkipsLLM(t *testing.T) {
	st, _ := store.Open(t.TempDir() + "/t.db")
	defer st.Close()
	_ = st.SetEngineConfig("interpret", "__enabled", "false", "") // global OFF

	srv := &Server{deps: Deps{
		Bus: bus.New(), Store: st,
		Summarizer: &fakeHeadless{out: `{"kind":"text","prompt":"e agora?","options":[]}`},
	}, interpret: newInterpretCache()}

	snap := &agui.Snapshot{
		SessionID: "s1", State: agui.StateAwaiting, Message: "Quer continuar?",
		History: []agui.HistoryLine{{Role: "assistant", Text: "Quer continuar?"}},
	}
	srv.attachInterpretation(snap)

	time.Sleep(50 * time.Millisecond)
	got, _ := st.ListEngineLog(10)
	if len(got) != 0 {
		t.Fatalf("interpret OFF não deveria rodar IA; veio %d", len(got))
	}
}
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/httpapi/ -run TestAttachInterpretation_DisabledSkipsLLM -v`
Expected: FAIL — auditoria gravada.

- [ ] **Step 3: Inserir o gate**

Em `internal/httpapi/interaction_interpret.go`, no início de `attachInterpretation`, logo após o `if s.deps.Summarizer == nil || ... { return }` existente, inserir:

```go
	// toggle de custo: interpretação é só-global (default ON).
	if s.deps.Store != nil && !s.deps.Store.EngineEnabled("interpret", "", true) {
		return
	}
```

- [ ] **Step 4: Rodar e ver passar (e suíte)**

Run: `go test ./internal/httpapi/ -run TestAttachInterpretation -v && go test ./internal/httpapi/`
Expected: PASS. O teste do Plano 2 `TestAttachInterpretation_LogsAudit` usa default ON (sem config) → continua passando. Se houver regressão, garantir que ele não setou `interpret` OFF.

- [ ] **Step 5: Commit**

```bash
git add internal/httpapi/interaction_interpret.go internal/httpapi/interaction_interpret_test.go
git commit -m "feat(toggle): gate da interpretacao (interpret) global por EngineEnabled"
```

---

### Task 4: Endpoint de leitura `GET /api/engines/{id}/enabled`

**Files:**
- Modify: `internal/httpapi/engines.go` (adicionar handler em `routesEngines`)
- Test: `internal/httpapi/engines_test.go` (criar, ou adicionar se existir)

**Interfaces:**
- Produces: `GET /api/engines/{id}/enabled?session_id=<sid>&default=<true|false>` → `{"enabled": bool}`. `default` ausente trata `summary`→false, `interpret`→true; qualquer outro id → false. (Escrita reaproveita o `PUT /api/engines/{id}/config` já existente, passando `project_id="session:<sid>"` para escopo de sessão ou `""` para global.)

- [ ] **Step 1: Escrever teste (falha)**

Criar `internal/httpapi/engines_test.go` (ou adicionar a um existente):

```go
package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func TestEnginesEnabledEndpoint(t *testing.T) {
	st, _ := store.Open(t.TempDir() + "/t.db")
	defer st.Close()
	_ = st.SetEngineConfig("summary", "__enabled", "true", "session:s1")

	srv := &Server{deps: Deps{Store: st}, mux: http.NewServeMux()}
	srv.routesEngines()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/engines/summary/enabled?session_id=s1&default=false", nil)
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if !body.Enabled {
		t.Fatalf("esperava enabled=true para a sessão s1")
	}
}
```

> Antes de implementar: confirmar em `engines.go`/`server.go` que `Server` tem
> campo `mux *http.ServeMux` e que `routesEngines` registra nele (visto no
> código atual). Ajustar a construção do `Server` no teste se a assinatura
> divergir.

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/httpapi/ -run TestEnginesEnabledEndpoint -v`
Expected: FAIL — rota inexistente (404), `status 404`.

- [ ] **Step 3: Adicionar o handler**

Em `internal/httpapi/engines.go`, dentro de `routesEngines`, adicionar (após o handler de `activity`):

```go
	s.mux.HandleFunc("GET /api/engines/{id}/enabled", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		sessionID := r.URL.Query().Get("session_id")
		// default por motor: summary OFF, interpret ON; query "default" sobrepõe.
		def := id == "interpret"
		if d := r.URL.Query().Get("default"); d != "" {
			def = d == "true"
		}
		writeJSON(w, http.StatusOK, map[string]bool{
			"enabled": s.deps.Store.EngineEnabled(id, sessionID, def),
		})
	})
```

- [ ] **Step 4: Rodar e ver passar (e suíte)**

Run: `go test ./internal/httpapi/ -run TestEnginesEnabledEndpoint -v && go test ./internal/httpapi/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/httpapi/engines.go internal/httpapi/engines_test.go
git commit -m "feat(api): GET /api/engines/{id}/enabled resolve toggle por sessao"
```

---

### Task 5: Cliente `api.ts` — ler/escrever o toggle

**Files:**
- Modify: `web/src/api.ts` (adicionar duas funções no fim do arquivo)

**Interfaces:**
- Produces:
  - `getEngineEnabled(id: string, sessionId?: string, def?: boolean): Promise<boolean>`
  - `setEngineEnabled(id: string, enabled: boolean, sessionId?: string): Promise<void>` — escreve `__enabled` no escopo `session:<id>` (se `sessionId`) ou global (`""`).

- [ ] **Step 1: Adicionar as funções**

No fim de `web/src/api.ts`:

```ts
// Toggle de custo dos motores de IA da Home (summary/interpret). scope vazio =
// global; sessionId = override por-miniatura ("session:<id>").
export async function getEngineEnabled(id: string, sessionId?: string, def?: boolean): Promise<boolean> {
  const qs = new URLSearchParams();
  if (sessionId) qs.set('session_id', sessionId);
  if (def !== undefined) qs.set('default', String(def));
  const r = await fetch(`/api/engines/${id}/enabled?${qs.toString()}`);
  const body = await r.json();
  return !!body.enabled;
}

export async function setEngineEnabled(id: string, enabled: boolean, sessionId?: string): Promise<void> {
  await fetch(`/api/engines/${id}/config`, {
    method: 'PUT', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      key: '__enabled',
      value: enabled ? 'true' : 'false',
      project_id: sessionId ? `session:${sessionId}` : '',
    }),
  });
}
```

- [ ] **Step 2: Type-check**

Run: `cd web && npx tsc -b`
Expected: PASS (No errors).

- [ ] **Step 3: Commit**

```bash
git add web/src/api.ts
git commit -m "feat(api-client): get/setEngineEnabled para toggles de custo"
```

---

### Task 6: `TerminalCard` — switch por-miniatura + cauda congelada (OFF)

**Files:**
- Modify: `web/src/pages/Home.tsx` (buscar e passar o estado de `summary` por sessão)
- Modify: `web/src/components/TerminalCard.tsx` (switch + render OFF)

**Interfaces:**
- Consumes: `getEngineEnabled('summary', id, false)`, `setEngineEnabled('summary', enabled, id)`.
- Produces: prop nova de `TerminalCard`: `summaryEnabled: boolean`, `onToggleSummary: (enabled: boolean) => void`.

- [ ] **Step 1: Home busca o estado de summary por sessão**

Em `web/src/pages/Home.tsx`, importar as funções e adicionar estado + loader. Após a linha `import { listSuggestions, getInteraction } from '../api';`, trocar por:

```tsx
import { listSuggestions, getInteraction, getEngineEnabled, setEngineEnabled } from '../api';
```

Após `const [snapshots, setSnapshots] = useState<Record<string, InteractionSnapshot>>({});` adicionar:

```tsx
  const [summaryOn, setSummaryOn] = useState<Record<string, boolean>>({});

  const loadSummaryFlags = useCallback(() => {
    const list = ids ? ids.split(',') : [];
    Promise.all(list.map((id) =>
      getEngineEnabled('summary', id, false).then((on) => [id, on] as const).catch(() => [id, false] as const),
    )).then((pairs) => {
      const next: Record<string, boolean> = {};
      for (const [id, on] of pairs) next[id] = on;
      setSummaryOn(next);
    });
  }, [ids]);

  const toggleSummary = useCallback((id: string, on: boolean) => {
    setSummaryOn((prev) => ({ ...prev, [id]: on })); // otimista
    setEngineEnabled('summary', on, id).then(loadSnapshots).catch(() => loadSummaryFlags());
  }, [loadSnapshots, loadSummaryFlags]);
```

E adicionar um effect para carregar os flags junto dos snapshots — após `useEffect(() => { loadSnapshots(); }, [loadSnapshots]);` inserir:

```tsx
  useEffect(() => { loadSummaryFlags(); }, [loadSummaryFlags]);
```

- [ ] **Step 2: Passar as props novas ao card**

Em `web/src/pages/Home.tsx`, no `<TerminalCard ... />`, adicionar as props:

```tsx
            <TerminalCard
              key={s.id}
              session={s}
              snapshot={snapshots[s.id]}
              awaiting={awaitingIds.has(s.id)}
              suggestions={pendingBySession[s.id] ?? 0}
              onActed={loadSnapshots}
              summaryEnabled={summaryOn[s.id] ?? false}
              onToggleSummary={(on) => toggleSummary(s.id, on)}
            />
```

- [ ] **Step 3: TerminalCard recebe as props e renderiza switch + cauda OFF**

Em `web/src/components/TerminalCard.tsx`, estender `Props`:

```tsx
interface Props {
  session: Session;
  snapshot?: InteractionSnapshot;
  awaiting: boolean;
  suggestions: number;
  onActed: () => void;
  // Toggle de custo do resumo de IA desta miniatura (Plano 3).
  summaryEnabled: boolean;
  onToggleSummary: (enabled: boolean) => void;
}
```

Trocar a assinatura do componente:

```tsx
export default function TerminalCard({ session, snapshot, awaiting, suggestions, onActed, summaryEnabled, onToggleSummary }: Props) {
```

Trocar o cálculo de `lines` (linha ~48) para escolher cauda congelada quando OFF:

```tsx
  // Resumo ligado: timeline narrada por IA. Desligado: cauda crua do histórico
  // (sem rolagem, só o final), custo zero.
  const lines = summaryEnabled
    ? timelineLines(session, snapshot, t('home.noProgress'))
    : frozenTail(snapshot, t('home.summaryOff'));
```

E adicionar, acima do componente, a função `frozenTail`:

```tsx
// frozenTail mostra as últimas linhas do histórico cru (o "final" da sessão),
// usado quando o resumo de IA está desligado para aquela miniatura.
function frozenTail(snapshot: InteractionSnapshot | undefined, fallback: string): string[] {
  const h = snapshot?.history ?? [];
  if (h.length === 0) return [fallback];
  return h.slice(-3).map((l) => l.text).filter((s) => s.trim().length > 0);
}
```

Adicionar o switch no rodapé do card — dentro de `<div className="tcard-foot">`, antes do `</div>` de fechamento do foot, inserir:

```tsx
        <label className="tcard-ai-toggle" title={t('home.summaryToggle', 'Resumo por IA (custa créditos)')}
          onClick={(e) => e.stopPropagation()}>
          <input
            type="checkbox"
            checked={summaryEnabled}
            onChange={(e) => onToggleSummary(e.target.checked)}
          />
          <span>IA</span>
        </label>
```

- [ ] **Step 4: Strings i18n (defaults inline já cobrem; opcional registrar)**

As chaves `home.summaryOff` e `home.summaryToggle` usam o 2º argumento de `t()` como default, então funcionam sem edição de locale. (Se o projeto exige chaves declaradas, adicioná-las aos arquivos de tradução em `web/src/` seguindo o padrão de `home.noProgress`.)

- [ ] **Step 5: Type-check, lint (sem novas regressões) e build**

Run: `cd web && npx tsc -b && npm run build`
Expected: PASS. (Rodar `npx eslint web/src/components/TerminalCard.tsx web/src/pages/Home.tsx web/src/api.ts` e confirmar que NÃO há erros novos nesses arquivos especificamente — o repo tem baseline de lint pré-existente em outros arquivos, que não conta.)

- [ ] **Step 6: Commit**

```bash
git add web/src/components/TerminalCard.tsx web/src/pages/Home.tsx
git commit -m "feat(home): switch de resumo por miniatura + cauda congelada quando OFF"
```

---

### Task 7: Controle global mínimo em Settings (summary OFF / interpret ON)

**Files:**
- Modify: `web/src/pages/Settings.tsx` (nova seção "Inteligência & custo" na aba Geral)

**Interfaces:**
- Consumes: `getEngineEnabled('summary', undefined, false)`, `getEngineEnabled('interpret', undefined, true)`, `setEngineEnabled('summary'|'interpret', bool)`.

> Nota de escopo: esta é a versão **mínima** do controle global (dois switches).
> A UI completa "Inteligência & custo" com escolha de harness/modelo e ciência de
> auditoria é o Plano 4.

- [ ] **Step 1: Importar e adicionar estado**

Em `web/src/pages/Settings.tsx`, na linha 3, acrescentar os imports:

```tsx
import { getSettings, putSettings, listProjects, listSessions, getEngineEnabled, setEngineEnabled } from '../api';
```

Após `const [tab, setTab] = useState('geral');` adicionar:

```tsx
  const [summaryGlobal, setSummaryGlobal] = useState(false);
  const [interpretGlobal, setInterpretGlobal] = useState(true);
  useEffect(() => {
    getEngineEnabled('summary', undefined, false).then(setSummaryGlobal).catch(() => {});
    getEngineEnabled('interpret', undefined, true).then(setInterpretGlobal).catch(() => {});
  }, []);
  const toggleSummaryGlobal = (on: boolean) => { setSummaryGlobal(on); setEngineEnabled('summary', on); };
  const toggleInterpretGlobal = (on: boolean) => { setInterpretGlobal(on); setEngineEnabled('interpret', on); };
```

- [ ] **Step 2: Renderizar a seção na aba Geral**

Localizar, em `Settings.tsx`, o bloco que renderiza a aba `geral` (onde aparece `retentionDays`/layout). Logo após o card de configurações gerais existente, dentro do `{tab === 'geral' && (...)}`, adicionar um novo card:

```tsx
          <div className="card" style={{ maxWidth: '760px', marginTop: '1rem' }}>
            <h2 style={{ marginTop: 0 }}>{t('settings.aiCostTitle', 'Inteligência & custo')}</h2>
            <p style={{ color: 'var(--muted)' }}>
              {t('settings.aiCostHint', 'Estes recursos usam IA e consomem créditos. Toda execução é auditada (prompt e resposta) na aba Atividade.')}
            </p>
            <label style={{ display: 'flex', gap: '0.5rem', alignItems: 'center', marginTop: '0.5rem' }}>
              <input type="checkbox" checked={summaryGlobal} onChange={(e) => toggleSummaryGlobal(e.target.checked)} />
              <span>{t('settings.aiSummary', 'Resumo de progresso na Home (narração ao vivo das miniaturas)')}</span>
            </label>
            <label style={{ display: 'flex', gap: '0.5rem', alignItems: 'center', marginTop: '0.4rem' }}>
              <input type="checkbox" checked={interpretGlobal} onChange={(e) => toggleInterpretGlobal(e.target.checked)} />
              <span>{t('settings.aiInterpret', 'Interpretação para resposta (transforma a fala do agente em opções acionáveis)')}</span>
            </label>
          </div>
```

> Antes de editar: ler o JSX da aba `geral` em `Settings.tsx` para inserir o card
> no lugar certo (dentro do `{tab === 'geral' && (...)}`). Se a aba usar um
> fragmento, manter a estrutura.

- [ ] **Step 3: Type-check, lint e build**

Run: `cd web && npx tsc -b && npm run build`
Expected: PASS. (Confirmar via `npx eslint web/src/pages/Settings.tsx` que não há erros novos nesse arquivo.)

- [ ] **Step 4: Commit**

```bash
git add web/src/pages/Settings.tsx
git commit -m "feat(settings): toggles globais de resumo (OFF) e interpretacao (ON)"
```
