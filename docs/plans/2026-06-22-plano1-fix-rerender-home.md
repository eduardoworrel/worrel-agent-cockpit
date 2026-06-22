# Plano 1 — Fix do re-render da Home Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eventos de sessão (`session.titled`/`session.ended`) deixam de desmontar a árvore inteira da Home; o modal de iniciar sessão preserva o texto digitado, o drawer de sugestões preserva o estado de colapso e a tela não pisca.

**Architecture:** O gate de tela em branco (`App.tsx`, `if (loading) return <div className="app-layout" />`) hoje é acionado por `setLoading(true)` em todo `reload()`. Separamos **carga inicial** (única, mostra blank) de **refetch em background** (mantém dados antigos montados). `useAppState` passa a expor `initialLoading` em vez de `loading`, e o refetch nunca toca esse flag. Também tornamos `setAwaitingIds` idempotente para evitar re-render desnecessário.

**Tech Stack:** React 19, TypeScript, Vite. Sem framework de teste de frontend no projeto — verificação por `tsc -b`, `eslint` e checagem comportamental via Playwright (padrão do repo).

## Global Constraints

- Comentários e copy em **português** (padrão do repo).
- Frontend **sem dependências novas** — o projeto não tem runner de teste de FE e não vamos adicionar.
- Não alterar contratos de API nem backend neste plano.
- Interface pública de `useAppState` muda de `loading` → `initialLoading`; todo consumidor deve ser atualizado no mesmo commit.

---

### Task 1: `useAppState` separa carga inicial de refetch

**Files:**
- Modify: `web/src/shell/useAppState.ts` (todo o arquivo)

**Interfaces:**
- Consumes: `listProjects()`, `listSessions()`, `listActiveSessions()` de `../api` (inalterados).
- Produces: `interface AppState` com o campo renomeado `initialLoading: boolean` (era `loading: boolean`); demais campos iguais (`projects`, `wrapperSessions`, `liveIds`, `isEmpty`, `reload`). `reload()` continua `() => void`, mas **não** sinaliza mais carregamento de tela cheia.

- [ ] **Step 1: Reescrever `useAppState.ts`**

Substituir o conteúdo inteiro de `web/src/shell/useAppState.ts` por:

```ts
import { useCallback, useEffect, useState } from 'react';
import { listProjects, listSessions, listActiveSessions } from '../api';
import type { Project, Session } from '../api';

export interface AppState {
  // initialLoading só é true até a PRIMEIRA carga concluir. Refetchs em
  // background (reload) não tocam este flag — assim a árvore nunca é
  // desmontada por um evento de sessão (ver App.tsx).
  initialLoading: boolean;
  projects: Project[];
  // Sessões iniciadas no app (Mode "wrapper"); base para grouping e isEmpty.
  wrapperSessions: Session[];
  // IDs das sessões com processo vivo (bar "Ativas"). A sidebar mostra só estas.
  liveIds: Set<string>;
  isEmpty: boolean;
  reload: () => void;
}

// useAppState carrega projetos, sessões e as sessões vivas; decide o estado
// macro do shell. isEmpty = nenhum projeto E nenhuma sessão wrapper → onboarding.
export function useAppState(): AppState {
  const [initialLoading, setInitialLoading] = useState(true);
  const [projects, setProjects] = useState<Project[]>([]);
  const [wrapperSessions, setWrapperSessions] = useState<Session[]>([]);
  const [liveIds, setLiveIds] = useState<Set<string>>(new Set());

  // reload busca em background e atualiza os dados sem nunca sinalizar tela de
  // carregamento. Em falha mantém os dados anteriores montados (não zera a UI).
  const reload = useCallback(() => {
    Promise.all([listProjects(), listSessions(), listActiveSessions()])
      .then(([projs, sessions, active]) => {
        setProjects(projs);
        setWrapperSessions(sessions.filter((s) => s.mode === 'wrapper'));
        setLiveIds(new Set(active.map((s) => s.id)));
      })
      .catch(() => { /* refetch falhou: preserva o último estado bom */ })
      .finally(() => setInitialLoading(false));
  }, []);

  useEffect(() => {
    reload();
  }, [reload]);

  const isEmpty = projects.length === 0 && wrapperSessions.length === 0;
  return { initialLoading, projects, wrapperSessions, liveIds, isEmpty, reload };
}
```

- [ ] **Step 2: Verificar type-check (vai falhar em App.tsx, esperado)**

Run: `cd web && npx tsc -b`
Expected: FAIL — erros em `web/src/App.tsx` referenciando `loading` (que não existe mais em `AppState`). Isso confirma que o único consumidor é o App e que a Task 2 precisa atualizá-lo. (Se aparecer erro em qualquer outro arquivo além de `App.tsx`, esse arquivo também consome `loading` e deve entrar na Task 2.)

- [ ] **Step 3: Commit**

```bash
git add web/src/shell/useAppState.ts
git commit -m "fix(home): separa carga inicial de refetch em useAppState"
```

---

### Task 2: `App.tsx` gateia só na carga inicial e torna awaitingIds idempotente

**Files:**
- Modify: `web/src/App.tsx:48` (destructuring de `useAppState`)
- Modify: `web/src/App.tsx:184` (gate de tela)
- Modify: `web/src/App.tsx:80-91` (handler de `session.awaiting`/`busy`/`ended`)

**Interfaces:**
- Consumes: `AppState.initialLoading` da Task 1.
- Produces: nada novo (componente de topo).

- [ ] **Step 1: Trocar `loading` por `initialLoading` no destructuring**

Em `web/src/App.tsx`, linha 48, trocar:

```tsx
  const { loading, projects, wrapperSessions, liveIds, isEmpty, reload } = useAppState();
```

por:

```tsx
  const { initialLoading, projects, wrapperSessions, liveIds, isEmpty, reload } = useAppState();
```

- [ ] **Step 2: Gatear a tela em branco só na carga inicial**

Em `web/src/App.tsx`, linha 184, trocar:

```tsx
  if (loading) return <div className="app-layout" />;
```

por:

```tsx
  if (initialLoading) return <div className="app-layout" />;
```

- [ ] **Step 3: Tornar `setAwaitingIds` idempotente (evita re-render à toa)**

Em `web/src/App.tsx`, no bloco `if (ev.type === 'session.awaiting' || ...)` (linhas ~80-91), substituir a chamada `setAwaitingIds`:

```tsx
        setAwaitingIds((prev) => {
          const next = new Set(prev);
          if (ev.type === 'session.awaiting') next.add(sid);
          else next.delete(sid);
          return next;
        });
```

por:

```tsx
        setAwaitingIds((prev) => {
          const has = prev.has(sid);
          if (ev.type === 'session.awaiting') {
            if (has) return prev; // já marcado: não recria o Set
            const next = new Set(prev);
            next.add(sid);
            return next;
          }
          if (!has) return prev; // já ausente: não recria o Set
          const next = new Set(prev);
          next.delete(sid);
          return next;
        });
```

- [ ] **Step 4: Verificar type-check e lint**

Run: `cd web && npx tsc -b && npx eslint .`
Expected: PASS — sem erros de tipo nem de lint.

- [ ] **Step 5: Build de produção**

Run: `cd web && npm run build`
Expected: PASS — `tsc -b && vite build` conclui sem erros.

- [ ] **Step 6: Commit**

```bash
git add web/src/App.tsx
git commit -m "fix(home): gate de tela só na carga inicial + awaitingIds idempotente"
```

---

### Task 3: Verificação comportamental (Playwright) — modal preserva texto sob evento de sessão

**Files:**
- (Sem arquivos novos — verificação manual assistida por Playwright MCP, padrão do repo.)

**Interfaces:**
- Consumes: app rodando localmente (`worrel` servindo o frontend buildado, ou `cd web && npm run dev`).

> Nota: o projeto não possui runner de teste de FE (sem vitest/jest). Seguindo o
> padrão existente (Playwright MCP + screenshots), a verificação é comportamental.
> Se o time decidir adicionar cobertura automatizada de FE no futuro, isso é um
> sub-projeto próprio (instalar vitest + @testing-library/react + jsdom) e está
> **fora do escopo** deste plano.

- [ ] **Step 1: Subir o app**

Run: `cd web && npm run dev`
Expected: Vite serve em `http://localhost:5173` (proxy para a API do `worrel`). Garantir que há ao menos uma sessão de terminal viva (para a miniatura existir) — criar uma se necessário.

- [ ] **Step 2: Reproduzir o cenário do bug via Playwright**

Usando o Playwright MCP:
1. `browser_navigate` para a Home.
2. Clicar em "Nova sessão" para abrir o `NewSessionWizard`/modal.
3. `browser_type` um texto reconhecível no campo de prompt do modal (ex.: `"TEXTO-DE-TESTE-PERSISTE"`).
4. Aguardar (ou provocar) um evento `session.titled`/`session.ended` — basta deixar uma sessão viva avançar (o resumo de progresso emite `session.titled`), ou encerrar/iniciar outra sessão.
5. `browser_snapshot` e asserir que o campo do modal **ainda contém** `"TEXTO-DE-TESTE-PERSISTE"` e que o modal continua aberto.

Expected: o texto persiste; o modal não remonta; sem flash de tela em branco.

- [ ] **Step 3: Verificar o drawer de sugestões**

1. Colapsar o `SuggestionsDrawer` (botão de colapso).
2. Provocar outro `session.titled`/`session.ended`.
3. Asserir que o drawer **permanece colapsado**.

Expected: `collapsed` preservado.

- [ ] **Step 4: Commit (registro da verificação, se houver screenshots)**

```bash
git add -A
git commit -m "test(home): verificação Playwright do fix de re-render" --allow-empty
```
