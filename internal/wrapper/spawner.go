package wrapper

import (
	"fmt"
	"os"

	"github.com/google/uuid"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/workspace"
)

// autoReportInstructions são as instruções enxutas de auto-relato (spec §5)
// injetadas no system prompt das sessões wrapper.
const autoReportInstructions = `Você está numa sessão gerida pelo worrel. Use as ferramentas MCP do servidor "worrel" proativamente:
- Ao concluir uma tarefa significativa, chame report_task_completed com um resumo do que foi feito.
- Quando o usuário corrigir seu entendimento ou uma convenção, chame report_correction.
- Quando perceber um procedimento repetível e reutilizável, chame propose_skill (objetivo, inputs, passos, edge cases, critérios de conclusão).
- Quando uma skill existente precisar de ajuste, chame propose_skill_update.
Não invente projetos nem memórias: tudo vira sugestão pendente para o usuário aprovar.
- Quando precisar confirmar uma ação ou pedir uma escolha ao usuário, chame a tool ask_user (ela mostra um balão na interface e espera a resposta) em vez de assumir.`

// worrelOnboarding é a explicação VISÍVEL de como o worrel funciona, entregue
// como início do primer (prompt posicional → aparece no transcript). Diferente
// do autoReportInstructions (lista de comandos no system prompt), aqui o foco é
// dar ao agente o MODELO mental — por que reportar importa — para que ele use as
// tools por iniciativa própria, e não as trate como ruído.
const worrelOnboarding = `# Como esta sessão funciona (worrel)

Você está rodando dentro do **worrel**, um cockpit que observa esta sessão para destilar conhecimento reutilizável (skills e memórias) para o usuário. Pontos essenciais:

- **Nada que você reportar é aplicado automaticamente.** Todo relato vira uma **sugestão pendente** que o usuário revisa e aprova numa fila. Por isso reportar é barato e seguro — na dúvida, reporte.
- Reporte **ao longo** da sessão, no momento em que o sinal aparece — não espere o fim.

## Tools do servidor MCP "worrel" — use proativamente

Escrita (cada uma cria uma sugestão pendente):
- **report_task_completed** — ao concluir uma tarefa significativa. Resuma o que foi feito e como. → vira MEMÓRIA do projeto.
- **report_correction** — quando o usuário corrigir seu entendimento, uma convenção ou uma decisão. → vira uma correção.
- **propose_skill** — quando perceber um PROCEDIMENTO repetível (objetivo, inputs, passos, edge cases, critério de conclusão). → vira uma skill nova.
- **propose_skill_update** — quando uma skill existente precisar de ajuste ou variação.
- **append_memory_suggestion** — para registrar um fato/decisão/convenção isolada a lembrar.

Interação:
- **ask_user** — quando precisar confirmar uma ação ou pedir uma escolha: mostra um balão na interface e espera a resposta. Não assuma.

Orientação (somente leitura, use para se situar): list_projects, get_project, list_skills, get_skill, load_skill, get_memory.

Regras: não invente projetos nem memórias; emita UMA sugestão por padrão recorrente (não uma por ocorrência); o usuário sempre tem a palavra final.

---
`

// onboardingMarker é o cabeçalho que abre worrelOnboarding. Usado por deriveTitle
// para identificar (e pular) o evento do primer injetado ao derivar o título da
// sessão. Mantém-se em sincronia com a primeira linha de worrelOnboarding.
const onboardingMarker = "# Como esta sessão funciona (worrel)"

// PrependOnboarding antepõe o onboarding do worrel a um primer. Vazio → só o
// onboarding. Usado tanto no spawn normal quanto no handoff (que substitui o
// primer), garantindo que toda sessão wrapper comece explicando o worrel.
func PrependOnboarding(primer string) string {
	if primer == "" {
		return worrelOnboarding
	}
	return worrelOnboarding + "\n" + primer
}

// BuildSpawnOpts monta as opções de spawn a partir do estado persistido:
// gera/persiste o token MCP, monta primer (memória + skill opcional) e a URL.
// O cwd é resolvido pelo workspace gerenciado: sess.WorkspaceDir se preenchido,
// senão o workspace do projeto (via wm.SyncProject), senão um scratch por sessão.
func BuildSpawnOpts(st *store.Store, wm *workspace.Manager, sessionID string, port int, skillContent string) (adapter.SpawnOpts, error) {
	sess, err := st.GetSession(sessionID)
	if err != nil {
		return adapter.SpawnOpts{}, err
	}

	workdir := sess.WorkspaceDir
	primer := ""
	if sess.ProjectID != "" {
		proj, err := st.GetProject(sess.ProjectID)
		if err != nil {
			return adapter.SpawnOpts{}, err
		}
		// cwd = workspace gerenciado do escopo, com symlinks p/ as pastas reais
		ws, err := wm.SyncProject(proj.Slug, proj.Dirs)
		if err != nil {
			return adapter.SpawnOpts{}, err
		}
		workdir = ws
		mem, err := st.GetMemory(sess.ProjectID)
		if err != nil {
			return adapter.SpawnOpts{}, err
		}
		primer = mem.Content
	} else if workdir == "" {
		// não-classificada sem workspace definido: cria scratch
		ws, err := wm.ScratchWorkspace(sessionID)
		if err != nil {
			return adapter.SpawnOpts{}, err
		}
		workdir = ws
	}
	if skillContent != "" {
		if primer != "" {
			primer += "\n\n"
		}
		primer += "## Skill a executar\n" + skillContent +
			"\n\nSiga a skill. Pergunte ao usuário os inputs declarados antes de começar."
	}

	token := uuid.NewString()
	if err := st.SetSessionMCPToken(sessionID, token); err != nil {
		return adapter.SpawnOpts{}, err
	}

	selfExe, _ := os.Executable()
	return adapter.SpawnOpts{
		SessionID:    sessionID,
		WorkingDir:   workdir,
		Primer:       PrependOnboarding(primer),
		SystemAppend: autoReportInstructions,
		MCPURL:       fmt.Sprintf("http://127.0.0.1:%d/mcp?s=%s", port, token),
		SelfExe:      selfExe,
		Port:         port,
	}, nil
}
