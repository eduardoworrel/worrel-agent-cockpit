# Migração Antigravity CLI + handoff com dados próprios — Design

Data: 2026-06-23
Status: aguardando revisão

## Contexto

O Google substituiu o **Gemini CLI** pelo **Antigravity CLI** (binário `agy`) em
2026-05-19. O worrel tem hoje um adapter `internal/adapter/gemini` que dirige o
binário `gemini`. Queremos:

1. Substituir o gemini legado pelo antigravity, compatibilizando com a estrutura
   de adapters do worrel.
2. Melhorar o modo "nova sessão" para lembrar o último provider/projeto usado.
3. (Levantado durante o brainstorming) Parar de "invadir" os arquivos do CLI no
   handoff de sessões PTY — passar a usar os dados que o próprio worrel observa.

O trabalho é decomposto em três sub-projetos independentes (arquivos
majoritariamente disjuntos), executáveis em paralelo via worktrees, com **P1 na
frente**.

## Fatos reais do `agy` (v1.0.10, verificados na máquina)

| Aspecto | Gemini legado | Antigravity (`agy`) |
|---|---|---|
| Binário | `gemini` | `agy` (em `~/.local/bin`) |
| Versão | `gemini --version` | `agy --version` → `1.0.10` |
| Headless | `gemini -p … --output-format json` → campo `response` | `agy -p "…"` → **texto puro** (NÃO existe `--output-format`); flags `--print-timeout`, `--model "Nome"` |
| Interativo + primer | `-i "<primer>"` | `-i "<primer>"` (idêntico: `--prompt-interactive`) |
| Auto-aprovar | hook BeforeTool | `--dangerously-skip-permissions` |
| Modelos | `-m <id>` | `--model "Gemini 3.1 Pro (Low)"`; `agy models` lista (Gemini 3.x, Claude 4.6, GPT-OSS) |
| MCP | env `GEMINI_CLI_SYSTEM_SETTINGS_PATH` → arquivo temp | `~/.gemini/antigravity-cli/mcp_config.json` (global) ou `.agents/mcp_config.json` (workspace); remoto exige campo **`serverUrl`** (não `httpUrl`). **Sem env-override confirmado.** |
| Hooks | `BeforeTool` no settings | Existem (`PreToolHooks`/`HookRegistry`/`context_engine_hook`) mas formato/local **não documentado**. |
| Histórico | `~/.gemini/tmp/<id>/logs.json` + `chats/*.json` (JSON legível) | `~/.gemini/antigravity-cli/conversations/<uuid>.db` (SQLite; `steps.step_payload` em **protobuf**) |
| Env injetadas | — | `ANTIGRAVITY_AGENT=1`, `ANTIGRAVITY_SIDECAR_WEB_PORT`, `INJECTED_PROJECT_ID`; projetos em `~/.gemini/config/projects/<uuid>.json` |

## Decisões tomadas no brainstorming

- **Remover o gemini de vez** (não manter como provider). ID novo `antigravity`.
- **Transcript do agy**: não invadir os arquivos do CLI (degradação graciosa),
  porque o caminho certo é o worrel persistir sua própria observação (P2).
- **Análise retroativa**: hoje está **órfã** (só sobraram strings de i18n e os
  blocos de adapter; sem rota HTTP nem UI). Vira sub-projeto próprio (P3).
- Paralelizar P1/P2/P3 (worktrees), P1 primeiro.

---

## P1 — Migração Antigravity + ordenação da nova sessão

### P1.a — Ordenação no NewSessionWizard (Tarefa 1)

Arquivo: `web/src/components/NewSessionWizard.tsx` (+ um hook utilitário).

- Novo módulo `web/src/lastUsed.ts`: lê/grava em `localStorage` um mapa
  `{ "provider:<id>": epochMs, "project:<id>": epochMs }`.
  - `markUsed(kind, id)` grava `Date.now()`.
  - `orderBy(kind, items, getId)` devolve os itens ordenados por timestamp desc
    (itens sem registro vão para o fim, preservando a ordem de chegada).
- **Provider**: em vez de `if (present[0]) setAdapterId(present[0].id)`, ordena
  `present` por `orderBy('provider', …)` e auto-seleciona o topo.
- **Projetos**: ordena a lista renderizada no passo 1 por `orderBy('project', …)`
  **sem** auto-seleção (o usuário escolhe). A opção "sem projeto" e o "+ novo
  projeto" permanecem nas posições fixas (topo/fundo).
- Em `start()`: `markUsed('provider', adapterId)` e, se `projectId`,
  `markUsed('project', projectId)`.

Testes: unit do `lastUsed.ts` (ordenação estável, ausência de registro,
timestamps). 

### P1.b — Adapter `antigravity`

Novo pacote `internal/adapter/antigravity/` (espelha a estrutura do gemini):

- `antigravity.go`:
  - `ID() => "antigravity"`.
  - `Detect()`: `exec.LookPath("agy")` + `agy --version` (regex `\d+\.\d+\.\d+`).
  - `Capabilities()`: `Caps{Hooks: false, Headless: true, OwnSessionID: false,
    ContextMeasured: false}` — Hooks/transcript degradados até P2/spike (ver
    "Riscos").
  - `BuildInteractive(opts)`: `agy` com `-i <primer>` (igual ao gemini). Injeção
    de MCP: ver "Riscos / Spike MCP" — na ausência de solução segura, **não**
    injeta MCP (degradação), e o comentário do código documenta isso.
  - `RunHeadless(ctx, prompt, opts)`: `agy -p <prompt>`; `--model opts.Model` se
    não-vazio; saída é **texto puro** (sem desembrulho JSON — retorna trim direto,
    com `--print-timeout` defensivo).
  - `DiscoverSessions/ReadTranscript` → `ErrNotSupported`.
  - `ContextUsage` → `(0,0,false)`.
  - `ModelLister.ListModels`: `agy models` (uma linha por modelo).
- `antigravity_test.go`: Detect (presente/ausente), buildargs interativo/headless,
  ListModels parse.

### P1.c — Remoção do gemini e wiring

- Deletar `internal/adapter/gemini/` inteiro (e seus testes).
- `cmd/worrel/main.go`: remover import e registro de `gemini`; adicionar
  `antigravity.New()` e `reg.Register(ag)`.
- `internal/engine/engine.go:54`: trocar a entrada `{Value:"gemini", …}` por
  `{Value:"antigravity", Label:"Antigravity", Description:"Executa via Antigravity CLI (agy)."}`.
- `web/src/components/HomeEngineConfig.tsx:10`: `{ value:'antigravity', label:'Antigravity' }`.
- `web/src/session.tsx:48`: cor para `antigravity` (manter `gemini` mapeado por
  segurança de render de sessões antigas, sem custo).
- `web/src/pages/SessionRoute.tsx:15`: trocar `'gemini'` por `'antigravity'` na
  lista `legacyAdapters`.
- `internal/hookprompt/hookprompt.go` e `cmd/worrel/hook.go`: o formato `gemini`
  fica **sem uso** (Hooks=false no novo adapter). Decisão: **remover** o case
  `gemini` do hookprompt e a menção no help do `hook.go`, já que o gemini saiu.
  (Se a investigação de hooks do agy — P1 risco — achar um formato viável,
  adiciona-se um case `antigravity` num passo posterior.)

### P1 — Riscos / Spike (timeboxed antes de fechar P1.b)

1. **Injeção de MCP** (o ponto mais incerto): o agy não tem env-override
   confirmado equivalente ao `GEMINI_CLI_SYSTEM_SETTINGS_PATH`. Investigar, nesta
   ordem: (a) flags/env não documentadas (`agy --help`, strings do binário);
   (b) escrever `.agents/mcp_config.json` no workspace com `serverUrl`;
   (c) merge-e-restaura no `~/.gemini/antigravity-cli/mcp_config.json` com
   backup+cleanup. Se nenhuma for segura, **degrada sem MCP** (documentado).
2. **Hooks**: formato/local do hook do agy é desconhecido. Sem confirmação,
   `Caps.Hooks=false` (sem balão de permissão; usa `--dangerously-skip-permissions`
   conforme o `permMode`). Investigação registrada como follow-up.

---

## P2 — Handoff com dados próprios (refactor transversal)

Hoje: sessões **stream-json** (Claude) já persistem o histórico no store
(`internal/streamengine/engine.go` `persistLines`) — dados nossos. Sessões
**PTY** (codex, opencode, pidev, e o futuro agy) **não**: o `wrapper`
relê o arquivo do CLI via `ReadTranscript` → `ingestTranscript`
(`internal/wrapper/transcript.go`). Isso é a "invasão".

Objetivo: o `wrapper` persiste sua **própria** observação do PTY em
`transcript_events`, de modo que o handoff (`internal/handoff/handoff.go`) leia
sempre nossos dados, sem tocar nos arquivos do CLI.

- Capturar o stream do PTY (entrada do usuário + saída do agente), limpar ANSI, e
  apendar como `transcript_events` (role heurístico você/ai; `Kind="pty"`).
- Handoff: já lê `transcript_events` primeiro; o fallback `LiveReader`
  (`ReadTranscript`) deixa de ser necessário para PTY e pode ser removido/relegado.
- `ContextUsage`: PTY não traz contagem de token → trigger automático de ~80%
  degrada. Opções: (a) sumiço do trigger automático (handoff manual permanece);
  (b) heurística por contagem de caracteres/estimativa de tokens. Decisão fica
  para o spec próprio de P2.

Escopo e riscos (ruído de ANSI, segmentação de turnos no PTY) merecem spec
dedicado. **P2 é sub-projeto separado**, mas pode rodar em paralelo a P1.

## P3 — Aba de análise retroativa em Config

Estado atual: feature **órfã** — strings de i18n (`i18n.ts:148-243`, chave
`onboarding.analyzeHistory`), adapters com `DiscoverSessions/ReadTranscript`, mas
**sem rota HTTP** (`server.go` não registra `/api/retro*` nem
`/sessions/{id}/distill`, embora `api.ts` defina `distillSession`) e **sem UI**
(nem no `OnboardingWizard`, nem no `EmptyState`, nem em `Settings.tsx`).

Objetivo: criar uma aba "Análise retroativa" em `Settings.tsx` (hoje só
`geral`, abas de engine, `atividade`) que orquestra
`DiscoverSessions` → seleção/escopo → distilação, com rota(s) HTTP novas no
`internal/httpapi`. Para o `antigravity`, como `DiscoverSessions` é
`ErrNotSupported`, a aba simplesmente **não oferece** o agy como fonte (os demais
CLIs com observer continuam disponíveis).

Feature nova e substancial — **sub-projeto separado**, independente de P1/P2.

---

## Plano de execução

1. P1 (worktree) — primeiro; entrega a substituição do gemini + ordenação.
2. P2 (worktree) — em paralelo; refactor do handoff.
3. P3 (worktree) — em paralelo; aba de análise retroativa.

Cada sub-projeto terá seu próprio plano de implementação (writing-plans) e ciclo
de revisão. Este documento é o spec-mãe da decomposição; P1 está detalhado o
suficiente para virar plano imediatamente.
