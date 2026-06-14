# Worrel Agent Cockpit

> Camada de memória, organização e destilação de conhecimento sobre CLIs de codificação agêntica — roda inteiramente na sua máquina, usa só a sua assinatura.

O Worrel é uma aplicação web local que observa e organiza suas sessões de trabalho com CLIs agênticos (Claude Code, OpenCode e outros). Ele extrai padrões, decisões e correções dessas sessões e os transforma em artefatos persistentes — **Projetos**, **Memórias** e **Skills** — que podem ser injetados em novas sessões de qualquer CLI suportado. Tudo fica em banco local (SQLite); nenhum dado sai da sua máquina e nenhuma telemetria é enviada. O app não tem chave de API própria: toda inteligência é exercida via subscription que você já possui nos CLIs.

## Rodando

A forma mais rápida (sem clonar nada):

```bash
npx worrel@latest
```

Baixa o binário da sua plataforma, sobe o cockpit em `localhost:7717` e abre o
navegador. Sempre na última versão. Flags úteis:

- `npx worrel --no-open` — não abrir o navegador.
- `npx worrel --port 8080` — escolher a porta (cai na próxima livre se ocupada).
- `npx worrel --version` — versão instalada.

Plataformas: macOS (arm64/x64) e Linux (x64/arm64). Windows ainda não — em breve.

Para desenvolvimento, a partir do código:

```bash
make build && ./bin/worrel
```

---

## Principais recursos

### Multi-CLI com adaptadores

Suporte nativo a **Claude Code** e **OpenCode** via camada de adaptadores com interface comum. Cada adaptador declara como spawnar sessões interativas, injetar contexto, registrar o MCP server, ler transcripts existentes e executar modo headless. Novos CLIs podem ser adicionados sem alterar o núcleo; capacidades ausentes degradam graciosamente.

### Dois modos de operação simultâneos

- **Modo wrapper** — sessões iniciadas de dentro da UI com terminal embutido; memória do projeto e instruções de auto-relato são injetados no contexto inicial.
- **Modo observador** — o app monitora os históricos de sessão dos CLIs instalados (`~/.claude/projects/` e equivalentes) e importa sessões iniciadas fora dele.

### MCP server local

O app expõe um MCP server que qualquer agente pode acessar, mesmo iniciado fora do app: listar projetos, carregar memória, buscar e executar skills, reportar eventos, solicitar segredos (`get_secret`), obter resumo de sessão para handoff.

### Projetos como escopo de trabalho

Um projeto é uma unidade de escopo definida pelo usuário, não uma pasta. Pode abranger múltiplos repositórios; pastas são apenas heurística de detecção. Cada projeto possui: memória própria (Markdown estruturado e versionado), skills, cofre de segredos e histórico de sessões.

### Auto-relato e varredura diferida

O próprio agente se reporta via MCP durante a sessão (`report_task_completed`, `report_correction`, `propose_skill`). Periodicamente, uma sessão headless do CLI preferido do usuário varre transcripts acumulados, captura o que o auto-relato perdeu e consolida sugestões redundantes — tudo sem LLM enquanto possível (screening em fase 1) e consumindo sua quota só quando necessário (fase 2).

### Fila de sugestões com aprovação humana

Toda sugestão (criar projeto, adicionar à memória, criar skill, atualizar skill) aparece em fila revisável em tempo real. Nenhum artefato é criado ou alterado sem aprovação do usuário. A fila nunca bloqueia o trabalho; pode ser revisada ao final do dia ou em lote.

### Cofre de segredos (AES-256-GCM / Keychain)

Dois modos por segredo:

- **Valor** — armazenado criptografado localmente (senha mestra e/ou keychain do SO). Todo acesso via `get_secret` gera registro de auditoria. Política configurável por segredo: liberar sempre / aprovar por sessão / aprovar a cada acesso.
- **Receita** — o app guarda apenas a *instrução de obtenção* ("rode `op read …`", "está em `.env.local`") sem custodiar o valor.

Injeção como variáveis de ambiente no spawn do CLI é opcional, desabilitada por padrão e exige confirmação explícita.

### Handoff de contexto (~80%)

Quando uma sessão wrapper se aproxima do limite de contexto, o app oferece iniciar nova sessão com resumo de handoff estruturado: estado atual, decisões tomadas, próxima ação, caminhos que falharam, arquivos relevantes. A cadeia de sessões encadeadas fica visível na UI.

### Motor de evolução de skills

Skills são entidades versionadas com linhagem, métricas e ciclo de vida. Toda criação ou alteração é classificada em um dos três tipos:

| Tipo | Identificador | Descrição |
|------|--------------|-----------|
| **Aprendizado** | `learned` | Padrão novo extraído de sessão bem-sucedida; cria skill geração 1 |
| **Correção** | `correction` | Reparo de skill existente (passo desatualizado, API mudou, falha); mesma skill, nova geração |
| **Variante** | `variant` | Especialização ou fusão; cria skill nova com referência à(s) mãe(s) |

Cada geração persiste: `skill_id` estável, tipo, ids das mães, diff legível, snapshot completo, resumo, evidência (trecho do transcript), autoria e timestamp. Reverter = reativar geração anterior com um clique; nenhuma geração é sobrescrita.

**Métricas e saúde** — taxa de sucesso em janela móvel, tendência, usos recentes. Ao cruzar limiares de degradação (ex.: ≥2 falhas consecutivas), o motor gera proativamente uma sugestão `skill.correction` com diagnóstico.

**Modo automático** — opt-in por skill (`manual` / `auto-correção` / `auto-total`). Toda aplicação automática fica em aba "Aplicadas automaticamente" com reverter em um clique; degradação de saúde pós-auto rebaixa a política para `manual` automaticamente.

**Import/export SKILL.md** — compatibilidade com o padrão aberto de Agent Skills (diretório `SKILL.md` com frontmatter YAML), consumível pelo Claude Code e outros CLIs.

### Análise retroativa ("Analisar tudo")

Fluxo sob demanda que elimina o cold start: varre todo o histórico existente dos CLIs detectados, propõe projetos a partir do acervo e roda o pipeline completo de destilação sobre o passado.

- **Estágio 0 — Inventário** (local, sem LLM): contagens, períodos cobertos, estimativa de custo.
- **Estágio 1 — Escopo e orçamento**: o usuário escolhe CLIs, pastas, janela de tempo e máximo de invocações headless; execução é pausável, retomável e cancelável.
- **Estágio 2 — Clusterização de projetos**: heurísticas locais + análise headless refinam grupos e propõem mapa de projetos para aprovação.
- **Estágio 3 — Destilação retroativa**: pipeline v1+v2 roda por projeto aprovado, gerando sugestões tipadas em visão de revisão em lote separada da fila incremental.
- **Segredos no histórico**: detector de credenciais nos transcripts antigos gera sugestões de cofre; valores nunca aparecem em texto claro na UI; rejeitados entram em lista de supressão por hash.
- **Idempotência**: rodar duas vezes no mesmo período não duplica sugestões nem projetos.

### Retenção de dados

Transcripts brutos são descartados após X dias (padrão: 30, configurável). Artefatos destilados (memórias, skills, correções, logs de auditoria de segredos) são permanentes. Evidências de sugestões pendentes são preservadas mesmo após a expiração do transcript bruto.

---

## Como rodar

**Pré-requisitos**: Go ≥ 1.22, Node.js ≥ 20.

```bash
# 1. Build da UI React + binário Go
make build

# 2. Iniciar o servidor
./bin/worrel
```

O app fica disponível em **http://127.0.0.1:7717**.

Para usar o cofre de segredos em modo valor, defina a senha mestra antes de iniciar:

```bash
WORREL_MASTER_PASSWORD=<senha> ./bin/worrel
```

Para desenvolvimento com live-reload da UI:

```bash
make web   # build da UI
make run   # build completo + execução
go test ./...  # testes
```

---

## Arquitetura

Binário Go único com UI React embutida (assets copiados para `internal/httpapi/dist` no build). Banco de dados SQLite local; sem dependências de rede externas em runtime.

Pacotes principais em `internal/`:

| Pacote | Responsabilidade |
|--------|-----------------|
| `adapter/` | Camada de adaptadores multi-CLI (Claude Code, OpenCode) |
| `httpapi/` | API REST + servidor de assets da UI |
| `mcpserver/` | MCP server local |
| `store/` | Acesso ao banco SQLite |
| `distill/` | Motor de varredura e destilação |
| `skillpkg/` | Motor de evolução de skills (linhagem, métricas, import/export) |
| `vault/` | Cofre de segredos (criptografia, auditoria, políticas) |
| `handoff/` | Resumo e handoff de contexto entre sessões |
| `retro/` | Análise retroativa do acervo |
| `wrapper/` | Modo wrapper (spawn de CLI com terminal embutido) |
| `workspace/` | Gestão de projetos e workspace |
| `retention/` | Política de retenção de transcripts |
| `bus/` | Barramento de eventos interno |
| `mirror/` | Mirror de transcripts em arquivos |
| `apply/` | Aplicação de sugestões aprovadas |

O ponto de entrada é `cmd/worrel/`.

---

## Licença

MIT — intenção declarada; consulte o arquivo `LICENSE` na raiz do repositório.

---

## Créditos — Motor de Evolução de Skills

O motor de evolução de skills deste projeto foi **inspirado em conceitos** do projeto [OpenSpace](https://github.com/HKUDS/OpenSpace) (HKUDS, licença MIT).

**Não há uso de código, runtime ou nuvem do OpenSpace.** A implementação aqui é inteiramente independente.

Conceitos inspirados no OpenSpace:

- **Taxonomia de evolução tipada** — lá chamada FIX / DERIVED / CAPTURED; aqui renomeada para **Correção / Variante / Aprendizado**, com semântica adaptada ao contexto de observação de sessões;
- **Modelo de linhagem versionada** — diff legível e snapshot completo persistidos por geração, com evolução sempre aditiva e reversão não-destrutiva;
- **Screening em duas fases** — fase 1 via regras e heurísticas locais (sem LLM); fase 2 de confirmação e redação via LLM (headless), acionada apenas para candidatos que passaram na fase 1;
- **Monitoramento de métricas e saúde por skill** — taxa de sucesso em janela móvel, detecção de degradação, geração proativa de sugestão de correção sem ação do usuário.

Diferenças centrais desta implementação:

- **Aprendizado por observação das próprias sessões do usuário** — o motor extrai conhecimento dos transcripts e do auto-relato do agente de trabalho, não por delegação a um sistema externo;
- **Aprovação humana como padrão global** — nenhuma skill ou memória é criada ou alterada sem aprovação do usuário; o modo automático é opt-in estritamente por skill, com salvaguardas de reversão e auto-suspensão por degradação de saúde;
- **Inteligência exclusivamente via subscription dos CLIs do usuário** — Claude Code, OpenCode e outros CLIs que o usuário já possui autenticados; sem dependência de nuvem proprietária, chave de API do app ou execução de software de terceiros no pipeline.
