# worrel — agent cockpit

> Camada de memória para CLIs de codificação agêntica. Observa suas sessões (Claude Code, OpenCode, Gemini, Codex, Pi) e destila **Projetos**, **Memórias** e **Skills** reutilizáveis — 100% local, sem telemetria, usando só a sua assinatura.

![license](https://img.shields.io/badge/license-MIT-blue)
![local-first](https://img.shields.io/badge/local--first-no%20telemetry-2ea44f)
![platforms](https://img.shields.io/badge/macOS%20%C2%B7%20Linux-arm64%20%7C%20x64-555)
![made with](https://img.shields.io/badge/Go%20%2B%20React-single%20binary-orange)

```bash
npx worrel@latest          # baixa o binário, sobe em localhost:7717 e abre o navegador
```

`--no-open` não abre o navegador · `--port 8080` escolhe a porta · `--version` mostra a versão.
Plataformas: macOS (arm64/x64), Linux (x64/arm64). Windows em breve.

---

## O que é

Você já trabalha com CLIs agênticos. O worrel **observa essas sessões** (iniciadas dentro ou fora do app), extrai padrões, decisões e correções, e os transforma em artefatos persistentes que podem ser **injetados de volta** em qualquer CLI suportado. Tudo em SQLite local; nenhum dado sai da máquina; o app não tem chave de API própria — a inteligência roda via subscription que você já possui.

## Recursos

| Recurso | O que faz |
|---|---|
| **Multi-CLI** | Adaptadores p/ Claude Code, OpenCode, Gemini, Codex e Pi — interface comum; capacidades ausentes degradam graciosamente; novos CLIs sem mexer no núcleo |
| **Wrapper + Observador** | Inicie sessões na UI com terminal embutido, **ou** deixe o app importar sessões que você rodou fora dele |
| **Projetos = escopo** | Unidade de trabalho definida por você (não pasta); abrange N repos. Tem memória, skills, segredos e sessões próprios |
| **MCP server local** | Qualquer agente acessa: listar projetos, carregar memória, buscar/rodar skills, reportar eventos, pedir segredos, resumir sessão p/ handoff |
| **Fila de sugestões** | Nada é criado/alterado sem sua aprovação; fila revisável em tempo real que nunca bloqueia o trabalho |
| **Cofre de segredos** | AES-256-GCM / Keychain. Modo **valor** (cifrado + auditoria por acesso) ou **receita** (só a instrução de obtenção). Injeção como env é opt-in |
| **Handoff de contexto** | Perto do limite (~80%), oferece nova sessão com resumo estruturado (estado, decisões, próxima ação, becos sem saída) |
| **Evolução de skills** | Skills versionadas com linhagem, métricas de saúde e reversão em 1 clique (ver abaixo) |
| **Análise retroativa** | Destila todo o histórico já existente dos seus CLIs sob demanda (ver abaixo) |
| **Retenção** | Transcripts brutos expiram (padrão 30d, configurável); artefatos destilados e auditoria são permanentes |

<details>
<summary><b>Evolução de skills</b> — tipos, linhagem e modo automático</summary>

Toda criação/alteração de skill é classificada:

| Tipo | ID | Descrição |
|---|---|---|
| **Aprendizado** | `learned` | Padrão novo de sessão bem-sucedida → skill geração 1 |
| **Correção** | `correction` | Reparo de skill existente → mesma skill, nova geração |
| **Variante** | `variant` | Especialização/fusão → skill nova referenciando a(s) mãe(s) |

Cada geração persiste `skill_id` estável, tipo, mães, diff legível, snapshot, evidência e autoria — **nada é sobrescrito**; reverter = reativar geração anterior.

- **Saúde**: taxa de sucesso em janela móvel; ao degradar (ex.: ≥2 falhas seguidas) o motor propõe `correction` proativamente.
- **Modo automático**: opt-in por skill (`manual` / `auto-correção` / `auto-total`), com aba dedicada, reversão em 1 clique e auto-rebaixamento p/ `manual` se a saúde cair.
- **SKILL.md**: import/export no padrão aberto de Agent Skills (frontmatter YAML), consumível por Claude Code e outros.
</details>

<details>
<summary><b>Análise retroativa</b> — "Analisar tudo" em 4 estágios</summary>

| Estágio | LLM? | O que faz |
|---|---|---|
| 0 · Inventário | não | Contagens, períodos, estimativa de custo |
| 1 · Escopo & orçamento | não | Escolhe CLIs/pastas/janela e teto de invocações; pausável/retomável/cancelável |
| 2 · Clusterização | sim | Heurística por pasta + 1 chamada refina e propõe o mapa de projetos p/ aprovação |
| 3 · Destilação | sim | Pipeline por projeto aprovado → sugestões tipadas em revisão em lote |

Detecção de **segredos** em transcripts antigos (valores nunca em texto claro; rejeitados entram em supressão por hash) e **idempotência** (rodar 2× no mesmo período não duplica).
</details>

## Como funciona

Binário Go único com UI React embutida; SQLite local; zero dependência de rede em runtime. O agente se auto-reporta via MCP durante a sessão; periodicamente uma varredura diferida (screening local sem LLM → confirmação headless só quando necessário) captura o que passou e consolida redundâncias.

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
| `httpapi/` | API REST + assets da UI | `handoff/` | Handoff entre sessões |
| `mcpserver/` | MCP server local | `retro/` | Análise retroativa |
| `store/` | Banco SQLite | `wrapper/` | Spawn de CLI + terminal |
| `distill/` | Varredura e destilação | `workspace/` | Projetos e workspace |
| `skillpkg/` | Evolução de skills | `retention/` | Retenção de transcripts |
| `apply/` | Aplicação de sugestões | `bus/` · `mirror/` | Eventos · mirror de transcripts |

Ponto de entrada: `cmd/worrel/`.

## Licença

MIT — ver [`LICENSE`](LICENSE).

## Créditos

Motor de evolução de skills **inspirado em conceitos** do [OpenSpace](https://github.com/HKUDS/OpenSpace) (HKUDS, MIT) — taxonomia de evolução tipada, linhagem versionada, screening em duas fases e monitoramento de saúde. **Sem uso de código, runtime ou nuvem do OpenSpace**; implementação independente. Diferenças centrais: aprendizado por observação das próprias sessões do usuário, aprovação humana como padrão global, e inteligência exclusivamente via subscription dos CLIs.
