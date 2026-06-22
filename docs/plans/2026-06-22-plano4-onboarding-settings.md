# Plano 4 — Onboarding & Settings: Inteligência & custo Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** O usuário decide, com ciência, no onboarding e em Settings: ligar/desligar os motores de IA da Home (resumo OFF default, interpretação ON default), **escolher harness e modelo** deles, e ver que toda execução de IA é auditada (prompt+resposta). A escolha de harness/modelo passa a ser **realmente consumida** pelas bordas `summary`/`interpret`.

**Architecture:** As bordas `attachProgress`/`attachEngineSummary`/`attachInterpretation` deixam de usar sempre `s.deps.Summarizer` e passam a resolver harness/modelo de `engine_config` (sessão ⊕ global), escolhendo o adapter de `s.deps.Adapters` (fallback para `Summarizer`). Um endpoint `GET /api/engines/{id}/settings` devolve `{enabled, harness, model}` resolvido. Onboarding e Settings compartilham um componente `HomeEngineConfig` que renderiza os dois motores de borda (on/off + harness/modelo + nota de auditoria). Os três motores de destilação (memory/skill/friction) já têm essa UI via `EngineCard`.

**Tech Stack:** Go (`go test`), React 19/TS (Vite). Sem dependências novas.

## Global Constraints

- Comentários e copy em **português**.
- NÃO registrar `summary`/`interpret` no `engine.Registry` (evita que o scheduler os dispare e gere logs vazios) — eles continuam sendo bordas; a UI deles é dedicada.
- Defaults: `summary` OFF, `interpret` ON. Auditoria inegociável (Plano 2) — intacta.
- Harness/modelo desses dois motores são **globais** (não por-sessão); o on/off do `summary` continua tendo override por-miniatura (Plano 3) — não mexer nisso.
- Reusar `ModelPicker` de `EngineCard.tsx` e `HarnessOptions` (espelhar os valores do backend `internal/engine/engine.go:HarnessOptions`).

---

### Task 1: Bordas consomem harness/modelo configurados

**Files:**
- Modify: `internal/httpapi/interaction_summary.go` (helper + 2 call sites)
- Modify: `internal/httpapi/interaction_interpret.go` (1 call site)
- Test: `internal/httpapi/interaction_summary_test.go` (adicionar)

**Interfaces:**
- Produces: `func (s *Server) summarizerFor(engineID, sessionID string) (HeadlessLLM, adapter.HeadlessOpts)` — resolve `harness`/`model` de `engine_config` (sessão ⊕ global); se `harness` resolve para um adapter headless em `s.deps.Adapters`, usa-o; senão `s.deps.Summarizer`. `opts.Model` recebe o `model` configurado.

> Antes de editar: reler as 3 goroutines para confirmar os nomes locais
> (`prompt`, `id`) e que cada uma chama `s.deps.Summarizer.RunHeadless(ctx, prompt, adapter.HeadlessOpts{})`.

- [ ] **Step 1: Escrever teste (falha) — harness configurado é usado**

Adicionar a `internal/httpapi/interaction_summary_test.go`. Usar um fake adapter que registra em `adapter.Registry` e conta chamadas distintas do `fakeHeadless` default:

```go
func TestSummarizerFor_UsesConfiguredHarness(t *testing.T) {
	st, _ := store.Open(t.TempDir() + "/t.db")
	defer st.Close()
	_ = st.SetEngineConfig("summary", "harness", "claude-code", "")
	_ = st.SetEngineConfig("summary", "model", "claude-sonnet-4-6", "")

	reg := adapter.NewRegistry()
	chosen := &countingAdapter{} // adapter fake headless; ver nota abaixo
	reg.Register(chosen)

	srv := &Server{deps: Deps{
		Store: st, Adapters: reg,
		Summarizer: &fakeHeadless{out: "fallback"},
	}}
	llm, opts := srv.summarizerFor("summary", "")
	if llm != HeadlessLLM(chosen) {
		t.Fatal("deveria escolher o adapter configurado (claude-code)")
	}
	if opts.Model != "claude-sonnet-4-6" {
		t.Fatalf("model não propagado: %q", opts.Model)
	}
}

func TestSummarizerFor_FallsBackToSummarizer(t *testing.T) {
	st, _ := store.Open(t.TempDir() + "/t.db")
	defer st.Close()
	sum := &fakeHeadless{out: "fallback"}
	srv := &Server{deps: Deps{Store: st, Adapters: adapter.NewRegistry(), Summarizer: sum}}
	llm, opts := srv.summarizerFor("summary", "")
	if llm != HeadlessLLM(sum) {
		t.Fatal("sem harness configurado, deve cair no Summarizer")
	}
	if opts.Model != "" {
		t.Fatalf("sem model configurado, opts.Model deveria ser vazio: %q", opts.Model)
	}
}
```

> Nota: `countingAdapter` precisa satisfazer `adapter.Adapter` com `ID()=="claude-code"`,
> `Capabilities().Headless==true` e `RunHeadless(...)`. Ver `internal/httpapi/models_test.go`
> (`baseFakeAdapter`) — REUSAR esse fake se já fornecer ID/Capabilities/RunHeadless,
> ajustando o `ID()` para `"claude-code"`. Se não houver fake reaproveitável, criar
> um mínimo no arquivo de teste. Ler `internal/adapter/adapter.go` para a interface
> `Adapter` completa antes de escrever o fake.

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/httpapi/ -run TestSummarizerFor -v`
Expected: FAIL — `srv.summarizerFor undefined`.

- [ ] **Step 3: Implementar o helper**

Em `internal/httpapi/interaction_summary.go`, adicionar:

```go
// summarizerFor escolhe o executor headless do motor de borda a partir do
// harness/modelo configurados em engine_config (sessão ⊕ global). Se o harness
// resolve para um adapter headless registrado, usa-o; senão cai no Summarizer
// padrão. opts.Model carrega o modelo configurado (vazio = default do CLI).
func (s *Server) summarizerFor(engineID, sessionID string) (HeadlessLLM, adapter.HeadlessOpts) {
	get := func(key string) string {
		if sessionID != "" {
			if m, err := s.deps.Store.GetEngineConfig(engineID, "session:"+sessionID); err == nil {
				if v, ok := m[key]; ok && v != "" {
					return v
				}
			}
		}
		if m, err := s.deps.Store.GetEngineConfig(engineID, ""); err == nil {
			return m[key]
		}
		return ""
	}
	opts := adapter.HeadlessOpts{Model: get("model")}
	if h := get("harness"); h != "" && s.deps.Adapters != nil {
		if ad, ok := s.deps.Adapters.Get(h); ok && ad.Capabilities().Headless {
			return ad, opts
		}
	}
	return s.deps.Summarizer, opts
}
```

- [ ] **Step 4: Usar o helper nas 3 goroutines**

Em cada uma das três goroutines (as duas em `interaction_summary.go` e a de `interaction_interpret.go`), trocar:

```go
		out, err := s.deps.Summarizer.RunHeadless(ctx, prompt, adapter.HeadlessOpts{})
```

por (usando o `engineID` e `sessionID` corretos de cada borda — `"summary"`+`id` nas duas de summary; `"interpret"`+`""` na de interpret):

```go
		llm, opts := s.summarizerFor("summary", id)
		out, err := llm.RunHeadless(ctx, prompt, opts)
```

e na borda de interpretação:

```go
		llm, opts := s.summarizerFor("interpret", "")
		out, err := llm.RunHeadless(ctx, prompt, opts)
```

> A guarda `if s.deps.Summarizer == nil` no topo de cada borda continua válida
> como "IA indisponível"; manter. `summarizerFor` só decide QUAL executor usar.

- [ ] **Step 5: Rodar e ver passar (e suíte)**

Run: `go test ./internal/httpapi/ -run TestSummarizerFor -v && go test ./internal/store/ ./internal/httpapi/`
Expected: PASS — incluindo os testes de auditoria e toggle dos Planos 2 e 3.

- [ ] **Step 6: Commit**

```bash
git add internal/httpapi/interaction_summary.go internal/httpapi/interaction_interpret.go internal/httpapi/interaction_summary_test.go
git commit -m "feat(ai): bordas summary/interpret consomem harness/modelo configurados"
```

---

### Task 2: Endpoint `GET /api/engines/{id}/settings`

**Files:**
- Modify: `internal/httpapi/engines.go` (handler em `routesEngines`)
- Test: `internal/httpapi/engines_test.go` (adicionar)

**Interfaces:**
- Produces: `GET /api/engines/{id}/settings?session_id=<sid>` → `{"enabled": bool, "harness": string, "model": string}` resolvido (sessão ⊕ global ⊕ default). Default de `enabled`: `interpret`→true, senão false.

- [ ] **Step 1: Escrever teste (falha)**

Adicionar a `internal/httpapi/engines_test.go`:

```go
func TestEnginesSettingsEndpoint(t *testing.T) {
	st, _ := store.Open(t.TempDir() + "/t.db")
	defer st.Close()
	_ = st.SetEngineConfig("summary", "__enabled", "true", "")
	_ = st.SetEngineConfig("summary", "harness", "opencode", "")
	_ = st.SetEngineConfig("summary", "model", "anthropic/claude-sonnet-4-6", "")

	srv := &Server{deps: Deps{Store: st}, mux: http.NewServeMux()}
	srv.routesEngines()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/engines/summary/settings", nil)
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var body struct {
		Enabled bool   `json:"enabled"`
		Harness string `json:"harness"`
		Model   string `json:"model"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if !body.Enabled || body.Harness != "opencode" || body.Model == "" {
		t.Fatalf("settings resolvido errado: %+v", body)
	}
}
```

- [ ] **Step 2: Rodar e ver falhar**

Run: `go test ./internal/httpapi/ -run TestEnginesSettingsEndpoint -v`
Expected: FAIL — 404.

- [ ] **Step 3: Adicionar o handler**

Em `internal/httpapi/engines.go`, dentro de `routesEngines`, após o handler `/enabled` (Plano 3), adicionar:

```go
	s.mux.HandleFunc("GET /api/engines/{id}/settings", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		sessionID := r.URL.Query().Get("session_id")
		get := func(key string) string {
			if sessionID != "" {
				if m, err := s.deps.Store.GetEngineConfig(id, "session:"+sessionID); err == nil {
					if v, ok := m[key]; ok && v != "" {
						return v
					}
				}
			}
			if m, err := s.deps.Store.GetEngineConfig(id, ""); err == nil {
				return m[key]
			}
			return ""
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": s.deps.Store.EngineEnabled(id, sessionID, id == "interpret"),
			"harness": get("harness"),
			"model":   get("model"),
		})
	})
```

- [ ] **Step 4: Rodar e ver passar (e suíte)**

Run: `go test ./internal/httpapi/ -run TestEnginesSettingsEndpoint -v && go test ./internal/httpapi/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/httpapi/engines.go internal/httpapi/engines_test.go
git commit -m "feat(api): GET /api/engines/{id}/settings devolve enabled+harness+model"
```

---

### Task 3: Componente `HomeEngineConfig` (compartilhado onboarding/Settings)

**Files:**
- Create: `web/src/components/HomeEngineConfig.tsx`
- Modify: `web/src/api.ts` (adicionar `getEngineSettings` e `setEngineConfigValue`)

**Interfaces:**
- Produces:
  - `api.ts`: `getEngineSettings(id, sessionId?) : Promise<{enabled:boolean; harness:string; model:string}>` e `setEngineConfigValue(id, key, value) : Promise<void>` (escopo global).
  - Componente `HomeEngineConfig` que renderiza UM motor de borda (props `id`, `title`, `description`, `defaultOn`) com switch on/off + harness (pills) + modelo (reusa `ModelPicker`) + nota de auditoria.

- [ ] **Step 1: Adicionar funções no `api.ts`**

No fim de `web/src/api.ts`:

```ts
export async function getEngineSettings(id: string, sessionId?: string): Promise<{ enabled: boolean; harness: string; model: string }> {
  const qs = sessionId ? `?session_id=${encodeURIComponent(sessionId)}` : '';
  const r = await fetch(`/api/engines/${id}/settings${qs}`);
  const b = await r.json();
  return { enabled: !!b.enabled, harness: b.harness ?? '', model: b.model ?? '' };
}

export async function setEngineConfigValue(id: string, key: string, value: string): Promise<void> {
  await fetch(`/api/engines/${id}/config`, {
    method: 'PUT', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ key, value }), // project_id ausente → escopo global
  });
}
```

- [ ] **Step 2: Criar `HomeEngineConfig.tsx`**

```tsx
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { getEngineSettings, setEngineConfigValue, setEngineEnabled } from '../api';

// Harness selecionáveis — espelha internal/engine/engine.go:HarnessOptions.
const HARNESSES = [
  { value: '', label: 'Padrão' },
  { value: 'claude-code', label: 'Claude Code' },
  { value: 'opencode', label: 'opencode' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'codex', label: 'Codex' },
];

function ModelPicker({ harness, current, onSelect }: { harness: string; current: string; onSelect: (v: string) => void }) {
  const [models, setModels] = useState<string[]>([]);
  useEffect(() => {
    const id = harness || 'claude-code';
    fetch(`/api/adapters/${id}/models`).then((r) => r.json())
      .then((d) => setModels(d.models || [])).catch(() => setModels([]));
  }, [harness]);
  return (
    <select className="ec-input" value={current} onChange={(e) => onSelect(e.target.value)}>
      <option value="">Padrão do harness</option>
      {models.map((m) => <option key={m} value={m}>{m}</option>)}
    </select>
  );
}

// HomeEngineConfig: um motor de IA da Home (summary/interpret) com on/off,
// harness, modelo e ciência de auditoria. Usado no onboarding e em Settings.
export default function HomeEngineConfig({ id, title, description, defaultOn }: {
  id: string; title: string; description: string; defaultOn: boolean;
}) {
  const { t } = useTranslation();
  const [enabled, setEnabled] = useState(defaultOn);
  const [harness, setHarness] = useState('');
  const [model, setModel] = useState('');

  useEffect(() => {
    getEngineSettings(id).then((s) => { setEnabled(s.enabled); setHarness(s.harness); setModel(s.model); }).catch(() => {});
  }, [id]);

  const toggle = (on: boolean) => { setEnabled(on); setEngineEnabled(id, on); };
  const pickHarness = (h: string) => { setHarness(h); setModel(''); setEngineConfigValue(id, 'harness', h); setEngineConfigValue(id, 'model', ''); };
  const pickModel = (m: string) => { setModel(m); setEngineConfigValue(id, 'model', m); };

  return (
    <div className="card" style={{ marginTop: '1rem' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: '1rem' }}>
        <div>
          <h3 style={{ margin: 0 }}>{title}</h3>
          <p style={{ margin: '0.3rem 0 0', color: 'var(--muted)', fontSize: '0.85rem' }}>{description}</p>
        </div>
        <button type="button" role="switch" aria-checked={enabled}
          className={`ec-switch${enabled ? ' on' : ''}`} onClick={() => toggle(!enabled)}>
          <span className="ec-knob" />
        </button>
      </div>
      <fieldset style={{ border: 'none', margin: 0, padding: '0.8rem 0 0', opacity: enabled ? 1 : 0.5 }} disabled={!enabled}>
        <div style={{ display: 'grid', gridTemplateColumns: '160px 1fr', gap: '0.8rem', alignItems: 'center', marginBottom: '0.6rem' }}>
          <label style={{ fontWeight: 600, fontSize: '0.9rem' }}>{t('aiCfg.harness', 'Harness')}</label>
          <div className="ec-pills">
            {HARNESSES.map((h) => (
              <button key={h.value} type="button" className={`ec-pill${harness === h.value ? ' on' : ''}`}
                onClick={() => pickHarness(h.value)}>{h.label}</button>
            ))}
          </div>
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: '160px 1fr', gap: '0.8rem', alignItems: 'center' }}>
          <label style={{ fontWeight: 600, fontSize: '0.9rem' }}>{t('aiCfg.model', 'Modelo')}</label>
          <ModelPicker harness={harness} current={model} onSelect={pickModel} />
        </div>
      </fieldset>
      <p style={{ marginTop: '0.7rem', fontSize: '0.8rem', color: 'var(--muted)' }}>
        🔒 {t('aiCfg.audit', 'Toda execução desta IA é registrada (prompt e resposta) na aba Atividade — auditoria sempre ativa e não desligável.')}
      </p>
    </div>
  );
}
```

> A classe `ec-switch`/`ec-knob`/`ec-pill`/`ec-pills`/`ec-input` vem do CSS
> embutido em `EngineCard.tsx` (`EC_CSS`), que é injetado quando um `EngineCard`
> é montado. Como `HomeEngineConfig` pode aparecer SEM um `EngineCard` na mesma
> tela, reusar a aparência exige garantir o CSS. Solução: importar/duplicar as
> regras mínimas. Na prática, em Settings e no onboarding um `EngineCard` está
> presente na mesma página (motores de destilação), então o CSS já está no DOM.
> Confirmar visualmente; se faltar estilo, copiar as 6 regras `.ec-switch`,
> `.ec-knob`, `.ec-pill(s)`, `.ec-input` para um `<style>` local neste componente.

- [ ] **Step 3: Type-check**

Run: `cd web && npx tsc -b`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add web/src/api.ts web/src/components/HomeEngineConfig.tsx
git commit -m "feat(ui): componente HomeEngineConfig (on/off + harness/modelo + auditoria)"
```

---

### Task 4: Onboarding ganha o passo "Inteligência & custo"

**Files:**
- Modify: `web/src/components/OnboardingWizard.tsx`

**Interfaces:**
- Consumes: `HomeEngineConfig` (Task 3).

- [ ] **Step 1: Inserir um passo dedicado antes do resumo**

Em `web/src/components/OnboardingWizard.tsx`, importar o componente:

```tsx
import HomeEngineConfig from './HomeEngineConfig'
```

Reordenar os passos para inserir "Inteligência & custo" como o passo logo antes do resumo. Trocar o cálculo de passos:

```tsx
  // passos: 0 = boas-vindas; 1..N = motores de destilação; N+1 = IA da Home
  // (resumo/interpretação); N+2 = resumo final
  const total = items.length + 3
  const isWelcome = step === 0
  const isHomeAI = step === items.length + 1
  const isSummary = step === items.length + 2
  const engine = !isWelcome && !isHomeAI && !isSummary ? items[step - 1] : undefined
```

E adicionar o bloco de render do novo passo, após o bloco `{engine && (...)}`:

```tsx
        {isHomeAI && (
          <div>
            <h2>Inteligência da Home (custo)</h2>
            <p style={{ color: 'var(--muted)' }}>
              Estes dois recursos usam IA e consomem créditos. Você decide agora — pode mudar depois em Configurações.
            </p>
            <HomeEngineConfig id="summary" defaultOn={false}
              title="Resumo de progresso"
              description="Narra ao vivo o que cada miniatura da Home está fazendo. Desligado, a miniatura mostra a cauda crua do terminal (sem custo). Começa DESLIGADO." />
            <HomeEngineConfig id="interpret" defaultOn={true}
              title="Interpretação para resposta"
              description="Transforma a fala do agente em opções acionáveis para você responder. Barato e pontual. Começa LIGADO." />
          </div>
        )}
```

> Antes de editar: reler o arquivo (Plano 3/4 podem ter mudado a numeração). O
> ponto-chave é: o `engine` (motor de destilação) só é exibido nos passos
> `1..items.length`; o passo `items.length+1` é o novo `isHomeAI`; o
> `items.length+2` é o `isSummary`. Ajustar todas as comparações de índice e o
> botão "Concluir" (que aparece em `isSummary`).

- [ ] **Step 2: Type-check e build**

Run: `cd web && npx tsc -b && npm run build`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add web/src/components/OnboardingWizard.tsx
git commit -m "feat(onboarding): passo Inteligencia & custo (resumo/interpretacao)"
```

---

### Task 5: Settings usa `HomeEngineConfig` (substitui a seção mínima do Plano 3)

**Files:**
- Modify: `web/src/pages/Settings.tsx`

**Interfaces:**
- Consumes: `HomeEngineConfig` (Task 3).

- [ ] **Step 1: Substituir o card mínimo "Inteligência & custo" pelo componente rico**

Em `web/src/pages/Settings.tsx`:

1. Importar: `import HomeEngineConfig from '../components/HomeEngineConfig';`
2. Remover o estado/handlers mínimos adicionados pelo Plano 3 (`summaryGlobal`, `interpretGlobal`, `toggleSummaryGlobal`, `toggleInterpretGlobal` e o `useEffect` que os carrega) e o card de checkboxes simples.
3. No lugar do card removido (dentro de `{tab === 'geral' && (...)}`), inserir:

```tsx
          <div style={{ maxWidth: '760px', marginTop: '1rem' }}>
            <h2 style={{ marginBottom: '0.2rem' }}>{t('settings.aiCostTitle', 'Inteligência & custo')}</h2>
            <p style={{ color: 'var(--muted)', marginTop: 0 }}>
              {t('settings.aiCostHint', 'Recursos de IA da Home. Escolha harness e modelo. Toda execução é auditada.')}
            </p>
            <HomeEngineConfig id="summary" defaultOn={false}
              title={t('settings.aiSummary', 'Resumo de progresso')}
              description={t('settings.aiSummaryDesc', 'Narração ao vivo das miniaturas. Desligado mostra a cauda crua (sem custo).')} />
            <HomeEngineConfig id="interpret" defaultOn={true}
              title={t('settings.aiInterpret', 'Interpretação para resposta')}
              description={t('settings.aiInterpretDesc', 'Transforma a fala do agente em opções acionáveis.')} />
          </div>
```

> Antes de editar: reler a aba `geral` em `Settings.tsx` para localizar exatamente
> o card de checkboxes inserido pelo Plano 3 e removê-lo por completo (estado +
> JSX), substituindo pelo bloco acima. Garantir que não sobra referência a
> `summaryGlobal`/`interpretGlobal`.

- [ ] **Step 2: Type-check, lint (sem novas regressões) e build**

Run: `cd web && npx tsc -b && npm run build`
Expected: PASS. Rodar `npx eslint web/src/pages/Settings.tsx web/src/components/HomeEngineConfig.tsx web/src/components/OnboardingWizard.tsx web/src/api.ts` e confirmar ZERO erros novos nesses arquivos.

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/Settings.tsx
git commit -m "feat(settings): Inteligencia & custo com harness/modelo (HomeEngineConfig)"
```
