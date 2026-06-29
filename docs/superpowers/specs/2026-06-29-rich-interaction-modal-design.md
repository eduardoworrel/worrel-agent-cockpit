# Modal de interação rico — design

Data: 2026-06-29
Branch: `feat/rich-interaction-modal`

## Problema

O modal automático de interação hoje (1) nunca mostra de fato o pedido do
usuário — o `user_message` cru some no fluxo — e (2) exibe a última fala da IA em
markdown cru, pouco legível e sem condensação.

Três mudanças isoláveis, na ordem 1 → 2 → 3. Cada fase entrega valor sozinha e é
revertível sem afetar as anteriores.

## Padrões existentes reaproveitados

- Engines LLM `summary` e `interpret` (`internal/agui/*.go` + `internal/httpapi/interaction_*.go`):
  prompt puro/determinístico no pacote `agui`, `Parse*` tolerante a lixo, cache
  por sessão na borda HTTP (`*Cache` com `get`/`claim`/`store`/`release`),
  chamada LLM assíncrona via `summarizerFor(engineID, sessionID)`, auditoria
  `Store.LogEngineRun`, publicação de `interaction.changed` ao concluir, toggle
  `Store.EngineEnabled(engineID, sessionID, default)`.
- `Snapshot` (`internal/agui/agui.go`) e seu espelho TS `InteractionSnapshot`
  (`web/src/api.ts`).
- Render do modal em `web/src/components/InteractionPanel.tsx`.

## Mudança 1 — `request_summary` (bloco "Seu pedido")

Nova engine LLM `request_summary` que condensa fielmente a **última** mensagem de
texto do usuário (P2=B), sem inferir intenção.

- `internal/agui/request_summary.go`: `RequestSummaryPrompt(userMessage string) string`
  (puro) + `ParseRequestSummary(out string) string` (tolerante; fallback = texto
  limpo/truncado). O prompt pede 1 frase curta começando por "O usuário pediu…".
- `internal/httpapi/interaction_request_summary.go`: `requestSummaryCache`
  (chaveado pelo conteúdo do `user_message`, igual ao `interpretCache.forMsg`,
  regenera quando a última mensagem muda) + `attachRequestSummary(snap)`.
  Assíncrono, timeout, `LogEngineRun{EngineID:"request_summary", Trigger:"realtime"}`,
  publica `interaction.changed`. Respeita `EngineEnabled("request_summary", id, true)`.
- `Snapshot`: novo campo `RequestSummary string json:"request_summary,omitempty"`.
  Espelho TS `request_summary?: string`.
- Wiring em `interaction.go`: chamar `attachRequestSummary` no caminho do motor e
  no caminho `Build` (PTY/transcript).
- `InteractionPanel.tsx`: bloco "Seu pedido" renderiza `request_summary`; se
  ausente, fallback para `user_message` cru (que hoje some).

## Mudança 2 — `ask_html` (bloco "A IA espera de você")

Nova engine LLM `ask_html` que gera um documento HTML completo (estilo inline,
sem `<script>`, estilo deliberadamente NÃO fixado) apresentando o que a IA
espera. **Inclui também** o `response_widget` (mudança 3) na mesma chamada.

- `internal/agui/ask_html.go`:
  - `AskHTMLPrompt(expects string, context []HistoryLine) string` (puro). Pede ao
    LLM um JSON `{"html":"<doc completo>","widget":{...|null}}`.
  - tipo `AskHTML struct { HTML string; Widget *ResponseWidget }`.
  - tipo `ResponseWidget struct { Type string; Spec json.RawMessage }` — JSON
    livre, sem tipo fixo (experimento).
  - `ParseAskHTML(out string) AskHTML` tolerante; em falha → `HTML=""`,
    `Widget=nil` (dispara fallback).
- `internal/httpapi/interaction_ask_html.go`: `askHTMLCache` chaveado pelo
  conteúdo de `expects` (= `interrupt.prompt ?? message`). Isso dá edge-trigger
  natural (P3=A): novo episódio de awaiting tem conteúdo diferente → regenera;
  polls do mesmo awaiting reaproveitam. `attachAskHTML(snap)` só roda quando
  `snap.NeedsAttention()` (awaiting/interrupt) e `expects != ""`. Assíncrono,
  timeout, `LogEngineRun{EngineID:"ask_html", Trigger:"realtime"}` (input=prompt,
  output=HTML+widget cru), publica `interaction.changed`. Respeita
  `EngineEnabled("ask_html", id, true)`.
- `Snapshot`: `AskHTML string json:"ask_html,omitempty"` e
  `ResponseWidget *ResponseWidget json:"response_widget,omitempty"`. Espelho TS.
- `InteractionPanel.tsx`: o bloco "A IA espera de você" renderiza `ask_html` num
  `<iframe sandbox>` SEM `allow-same-origin` (isolamento total; nenhum
  `postMessage`). Altura por atributo simples.
- Fallback robusto: se `ask_html` falhar/der timeout/vier vazio → render markdown
  atual (`expects`). O HTML NUNCA bloqueia a abertura do modal nem o input.

## Mudança 3 — widget de resposta dinâmico (experimental/removível)

- Origem (P4=A): o `response_widget` vem da MESMA chamada `ask_html`.
- `web/src/components/ResponseWidget.tsx`: componente isolado com um `switch` por
  `type` (ex.: `range`, `options` custom; default → cai no form de texto). O
  iframe é só apresentação; a resposta acontece em React e usa o mesmo caminho de
  `sendPrompt`/choice já existente.
- Escopo (P4=A): widget só atua em awaiting livre / `text` / `choice`. Quando há
  `interrupt` de permissão de broker (`request_id` presente), mantém os botões
  fixos allow/deny — widget NÃO interfere.
- Flag de desligamento: const `RESPONSE_WIDGET_ENABLED` no front. "Se der ruim,
  deleta o `switch`/`ResponseWidget` e volta ao form de texto" — sem tocar nas
  mudanças 1 e 2.

## Testes

- Go: `Parse*` (request_summary/ask_html) com saídas LLM bem/mal-formadas;
  garantir que `attach*` nunca propaga erro pro snapshot (fallback) e que o cache
  evita regeração por poll.
- Frontend: render com/sem cada campo novo (fallbacks), iframe sandbox sem
  same-origin, widget desligado pela flag.

## Não-objetivos

- Não fixar o estilo do HTML (observar variação ao longo do tempo).
- Widget não cobre permissão de broker.
- Sem `postMessage`/`<script>` no iframe.
