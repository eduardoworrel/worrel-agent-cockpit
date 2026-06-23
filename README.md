# worrel — agent cockpit

> Camada de memória para CLIs de codificação agêntica. Observa suas sessões (Claude Code, OpenCode, Gemini, Codex, Pi) e destila **Projetos**, **Memórias** e **Skills** reutilizáveis — 100% local, sem telemetria, usando só a sua assinatura.

![release](https://img.shields.io/github/v/release/eduardoworrel/worrel-agent-cockpit)
![license](https://img.shields.io/badge/license-MIT-blue)
![local-first](https://img.shields.io/badge/local--first-no%20telemetry-2ea44f)
![platforms](https://img.shields.io/badge/macOS%20%C2%B7%20Linux-arm64%20%7C%20x64-555)
![made with](https://img.shields.io/badge/Go%20%2B%20React-single%20binary-orange)

```bash
npx worrel@latest          # baixa o binário, sobe em localhost:7717 e abre o navegador
```

`--no-open` não abre o navegador · `--port 8080` escolhe a porta · `--version` mostra a versão.
Dados em `~/.worrel` (SQLite). Plataformas: macOS (arm64/x64), Linux (x64/arm64). Windows em breve.

## O que é

Você já trabalha com CLIs agênticos. O worrel **observa essas sessões** — iniciadas dentro ou fora do app — extrai padrões, decisões e correções, e os transforma em artefatos persistentes que podem ser **injetados de volta** em qualquer CLI suportado. Tudo em SQLite local; nenhum dado sai da máquina; o app não tem chave de API própria — a inteligência roda via subscription que você já possui.

## Recursos

| Recurso | O que faz |
|---|---|
| **Multi-CLI** | Adaptadores p/ Claude Code, OpenCode, Gemini, Codex e Pi — interface comum; capacidades ausentes degradam graciosamente |
| **Sessão nativa** | Conduz o Claude Code pelo protocolo stream-json (sem MCP/hook/PTY): texto, tool-use e pedidos de permissão viram interação na Home |
| **Projetos = escopo** | Unidade de trabalho definida por você (não pasta); abrange N repos, com memória, skills, segredos e sessões próprios |
| **Motores de destilação** | Registry de motores declaráveis (memória/skill/atrito) com toggle, gatilho e prompts editáveis — nada roda sozinho por default |
| **Fila de sugestões** | Nada é criado/alterado sem sua aprovação; fila revisável que nunca bloqueia o trabalho |
| **MCP server local** | Qualquer agente acessa: listar projetos, carregar memória, buscar/rodar skills, reportar eventos, pedir segredos, resumir sessão p/ handoff |
| **Cofre de segredos** | AES-256-GCM / Keychain. Modo **valor** (cifrado + auditoria) ou **receita** (só a instrução de obtenção). Injeção como env é opt-in |
| **Handoff de contexto** | Perto do limite (~80%), oferece nova sessão com resumo estruturado (estado, decisões, próxima ação, becos sem saída) |
| **Evolução de skills** | Skills versionadas com linhagem, métricas de saúde e reversão em 1 clique (ver abaixo) |
| **Análise retroativa** | Destila o histórico já existente dos seus CLIs sob demanda (ver abaixo) |
| **Retenção** | Transcripts brutos expiram (padrão 30d, configurável); artefatos destilados e auditoria são permanentes |

### Motores de destilação

Cada motor (`internal/engine/*`) é declarativo: tem id, gatilho, prompts e config editáveis, e só executa quando habilitado (global ou por projeto). Disparo manual (sob demanda) ou automático via `scheduler`, que roda cada (motor, sessão) no máximo uma vez.

| Motor | O que destila |
|---|---|
| **memory** | Golden truths anti-erro do transcript (padrão erro→correção). Injeção `sempre` (primer) ou `sob demanda` (via MCP `get_memory`) |
| **skill** | Workflows dirigidos pelo usuário; acumula recorrência entre sessões e os matura em skills ou agentes |
| **friction** | Roteia sinais de atrito para memória / nova skill / refinar skill ou agente / saúde de skill |

Gatilhos disponíveis: `on_demand`, `realtime`, `periodic`, `project_open_close`, `agent_self`.

<details>
<summary><b>Evolução de skills</b> — tipos, linhagem e modo automático</summary>

Toda criação/alteração de skill é classificada:

| Tipo | ID | Descrição |
|---|---|---|
| **Aprendizado** | `learned` | Padrão novo de sessão bem-sucedida → skill geração 1 |
| **Correção** | `correction` | Reparo de skill existente → mesma skill, nova geração |
| **Variante** | `variant` | Especialização/fusão → skill nova referenciando a(s) mãe(s) |

Cada geração persiste `skill_id` estável, tipo, mães, diff legível, snapshot, evidência e autoria — **nada é sobrescrito**; reverter = reativar geração anterior. **Saúde**: taxa de sucesso em janela móvel; ao degradar, o motor propõe `correction`. **Modo automático**: opt-in por skill (`manual` / `auto-correção` / `auto-total`), com auto-rebaixamento p/ `manual` se a saúde cair. Import/export no padrão aberto de Agent Skills (`SKILL.md`, frontmatter YAML).
</details>

<details>
<summary><b>Análise retroativa</b> — "Analisar tudo" em 4 estágios</summary>

| Estágio | LLM? | O que faz |
|---|---|---|
| 0 · Inventário | não | Contagens, períodos, estimativa de custo |
| 1 · Escopo & orçamento | não | Escolhe CLIs/pastas/janela e teto de invocações; pausável/retomável/cancelável |
| 2 · Clusterização | sim | Heurística por pasta + 1 chamada refina e propõe o mapa de projetos p/ aprovação |
| 3 · Destilação | sim | Pipeline por projeto aprovado → sugestões tipadas em revisão em lote |

Detecção de **segredos** em transcripts antigos (valores nunca em texto claro; rejeitados entram em supressão por hash) e **idempotência** (rodar 2× no mesmo período não duplica). Chamadas headless do próprio worrel são reconhecidas (`internal/metasession`) e nunca re-observadas como trabalho do usuário.
</details>

## Como funciona

Binário Go único com UI React embutida; SQLite local; zero dependência de rede em runtime.

- A **sessão** é dirigida nativamente pelo CLI (`internal/streamengine`, protocolo stream-json) e exposta à Home por um contrato AG-UI (`internal/agui`): o que a IA disse/fez, o último pedido e qualquer pergunta bloqueante.
- **Permissões** de ferramenta chegam como balões na UI (`internal/ask`); o subcomando `worrel hook prompt` (`internal/hookprompt`) integra o mesmo fluxo a CLIs que usam hooks (Claude/Codex/Gemini).
- Os **motores** rodam sob demanda ou via `scheduler` sobre sessões encerradas; toda saída vira sugestão na fila, aprovada por você antes de virar artefato.

## Desenvolvimento

**Pré-requisitos:** Go ≥ 1.22, Node.js ≥ 20.

```bash
make build                       # UI React + binário Go
./bin/worrel                     # http://127.0.0.1:7717
WORREL_MASTER_PASSWORD=… ./bin/worrel   # habilita o cofre em modo valor
make run                         # build + run     ·     go test ./...
```

### Arquitetura (`internal/`)

| Pacote | Responsabilidade | Pacote | Responsabilidade |
|---|---|---|---|
| `adapter/` | Adaptadores multi-CLI | `vault/` | Cofre de segredos |
| `streamengine/` | Sessão nativa (stream-json) | `ask/` | Balões de confirmação/escolha |
| `agui/` | Contrato de interação da Home | `hookprompt/` | Subcomando `worrel hook prompt` |
| `engine/` | Registry de motores de destilação | `scheduler/` | Auto-execução dos motores |
| `prompts/` | Prompts default editáveis | `metasession/` | Detecção de chamadas do próprio app |
| `skillpkg/` | Evolução de skills | `apply/` | Aplicação de sugestões |
| `httpapi/` | API REST + assets da UI | `mcpserver/` | MCP server local |
| `store/` | Banco SQLite | `workspace/` | Projetos e workspace |
| `wrapper/` | Spawn de CLI + terminal | `handoff/` | Handoff entre sessões |
| `retention/` | Retenção de transcripts | `bus/` · `mirror/` | Eventos · mirror de transcripts |

Ponto de entrada: `cmd/worrel/`.

## Licença

MIT — ver [`LICENSE`](LICENSE).

## Créditos

Motor de evolução de skills **inspirado em conceitos** do [OpenSpace](https://github.com/HKUDS/OpenSpace) (HKUDS, MIT) — taxonomia de evolução tipada, linhagem versionada, screening em duas fases e monitoramento de saúde. **Sem uso de código, runtime ou nuvem do OpenSpace**; implementação independente. Diferenças centrais: aprendizado por observação das próprias sessões do usuário, aprovação humana como padrão global, e inteligência exclusivamente via subscription dos CLIs.
