# Design: Fix de re-render da Home + toggles de IA por custo + auditoria de IA

Data: 2026-06-22
Status: Aprovado (brainstorming)

## Contexto

A Home do cockpit mostra cada sessão de terminal viva como um card ("miniatura").
Duas funções de IA alimentam essa tela e custam dinheiro; além delas há motores de
destilação (memory, skill, friction) que também usam IA. Este design cobre três
frentes acordadas para serem tratadas juntas:

1. **Bug:** todo evento de sessão re-renderiza/desmonta a página inteira.
2. **Custo:** poder ligar/desligar os recursos de IA da Home (global e por-miniatura).
3. **Governança:** auditoria/explicabilidade inegociável de toda chamada de IA, com
   escolha de harness/modelo e ciência do usuário no onboarding.

## Frente 1 — Fix do re-render da Home

### Problema (causa-raiz confirmada)

- `App.tsx` trata eventos `session.titled` e `session.ended` chamando `reload()`
  (`App.tsx:97,102`).
- `reload()` (em `web/src/shell/useAppState.ts`) faz `setLoading(true)` antes do refetch.
- `App.tsx:184`: `if (loading) return <div className="app-layout" />`.
- Consequência: enquanto o refetch roda, **a árvore inteira é desmontada** — Home,
  `SuggestionsDrawer` e o modal `NewSessionWizard` — e remontada ao concluir.
- `session.titled` dispara durante uso normal do terminal (título derivado da 1ª
  mensagem), então parece que "todo evento do terminal" quebra tudo.
- Sintomas: modal de iniciar sessão perde o texto digitado (estado local remonta),
  `SuggestionsDrawer` reabre (`collapsed` volta a `false`), tela pisca.

> Observação: `interaction.changed` **não** é o culpado — ele fica isolado na Home
> (`Home.tsx:55-59`) e só atualiza `snapshots`. O culpado é o gate de `loading`
> sobre refetchs.

### Solução

- `useAppState` distingue **carga inicial** de **refetch em background**:
  - `initialLoading` só é `true` na primeira carga (ainda sem dados).
  - Refetchs (`reload()`) mantêm os dados anteriores montados e atualizam em background;
    não tocam em `initialLoading`.
- `App.tsx:184` passa a gatear a tela em branco apenas em `initialLoading`.
- `setAwaitingIds` não recria o `Set` quando o conteúdo não muda (evita re-render à toa).

### Critério de aceite / teste

- Teste RTL: com `NewSessionWizard` aberto e texto digitado, disparar `session.titled`
  → o texto persiste, o modal continua montado.
- Teste: `SuggestionsDrawer` com `collapsed=true` permanece colapsado após `session.ended`.

## Frente 2 — Toggles de IA da Home (controle de custo)

Há **duas funções de IA distintas** alimentando a Home (confirmado em `internal/agui`):

| Função | Arquivo | IA? | Frequência | Custo |
|---|---|---|---|---|
| `translator.Build` (snapshot estruturado) | `translator.go` | Não (determinístico) | toda render | zero |
| **Resumo de progresso** (`ProgressPrompt`/`ParseProgress`) | `summary.go` | Sim | contínua, enquanto a sessão vive | **dominante** |
| **Interpretação para resposta** (`InterpretPrompt`/`ParseInterpretation`) | `interpret.go` | Sim | event-driven (agente termina falando) | baixo |

### Motor "resumo de progresso" (summary)

- Vira um motor explícito com `engine_config` própria.
- `__enabled` **global** com default **OFF** + **override por-sessão/miniatura**.
  (a `engine_config` já é escopada por projeto; estender o escopo para `session` ou
  guardar override por sessão é decisão da fase de implementação.)
- UI: switch na própria `TerminalCard` (liga/desliga aquela miniatura), toggle global
  em Settings, e item no onboarding.
- **OFF:** a Home **não** chama o summary; `TerminalCard` renderiza a **cauda congelada**
  do último snapshot determinístico (`translator.Build`, custo zero), **sem rolagem**;
  se o conteúdo for grande, mostra apenas o **final**.
- **ON:** comportamento atual (narração ao vivo), agora sempre auditado.
- Harness/modelo escolhíveis (onboarding + Settings).

### Motor "interpretação para resposta" (interpret)

- `__enabled` **só global**, default **ON** (é o "core" que permite responder ao agente;
  é barato e event-driven, então não tem override por-card).
- **OFF global:** agente que encerra falando não vira UI acionável — o usuário responde
  olhando o terminal cru.
- Harness/modelo escolhíveis (onboarding + Settings).

## Frente 3 — Auditoria / explicabilidade inegociável

### Princípio

- Toda chamada de IA (summary, interpret, memory, skill, friction) **sempre** grava um
  registro. **Não existe switch que desligue o log.** O usuário tem ciência, não controle
  sobre o registro em si. O que é configurável é **se o motor roda** e **com qual harness/modelo**.

### Modelo de dados — estender o que já existe

O `engine_log` (`internal/store/engine_log.go`) já é o registro de explicabilidade:
uma linha por execução de motor, com `session_id` (a sessão de origem), `engine_id`,
`trigger`, `detail`, `created_at`; alimenta a aba "Atividade". **Não** se cria sistema
paralelo — estende-se essa tabela com duas colunas:

```go
type EngineLogEntry struct {
    ID          int64
    EngineID    string
    ProjectID   string
    SessionID   string  // já existe → a sessão que gerou aquilo
    Trigger     string
    Suggestions int
    Detail      string
    Input       string  // novo: prompt enviado à IA
    Output      string  // novo: resposta crua do modelo
    CreatedAt   int64
}
```

- Migração: `ALTER TABLE engine_log ADD COLUMN input TEXT; ADD COLUMN output TEXT;`
- `LogEngineRun` passa a gravar `input`/`output`. Motores/execuções heurísticas
  (`heuristic_only`) deixam vazio (não houve IA). Os cinco motores usam o mesmo caminho.
- `ListEngineLog` e a aba "Atividade" ganham os dois campos; abrir uma linha mostra
  input/output. O `session_id` já leva de volta à sessão.
- **Uma linha por execução = um par input/output** (cada run de IA faz uma chamada),
  consistente com o modelo atual.
- **Retenção:** o registro de auditoria **vive e morre exatamente como o `engine_log`
  vive hoje** — mesmo ciclo de vida, sem regra nova. (Na implementação, verificar se há
  poda existente e manter idêntico.)

## Frente 3 (cont.) — Onboarding & Settings

- Onboarding ganha o passo **"Inteligência & custo"**, ponto único onde a pessoa decide,
  com ciência:
  - lista os motores de IA (resumo, interpretação, memory, skill, friction);
  - on/off conforme as regras acima (resumo nasce OFF, interpretação ON);
  - **escolha de harness e modelo** por motor;
  - aviso de que toda chamada de IA é auditada (input/output) e por quê.
- Settings espelha o mesmo, editável depois; a aba "Atividade" exibe os registros
  (incluindo input/output) dos cinco motores.

## Fora de escopo

- Mudar o ciclo de vida/poda atual do `engine_log` (apenas seguir o existente).
- Auditoria de passos intermediários da IA além de input/output.
- Override por-card para o motor de interpretação.

## Resumo das decisões

| Tema | Decisão |
|---|---|
| Bug | Separar `initialLoading` de refetch; gate de tela só na carga inicial |
| Estado OFF da miniatura | Cauda congelada do `translator.Build`, sem rolagem, mostra o final |
| Default toggle resumo | OFF (global + por-miniatura) |
| Default toggle interpretação | ON (só global) |
| Auditoria | Inegociável; estende `engine_log` com `input`/`output` |
| Retenção da auditoria | Igual ao `engine_log` atual |
| Harness/modelo | Escolhidos no onboarding e em Settings, por motor |
| Onboarding | Passo "Inteligência & custo" com ciência + decisões |
