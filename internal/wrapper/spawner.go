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
		Primer:       primer,
		SystemAppend: autoReportInstructions,
		MCPURL:       fmt.Sprintf("http://127.0.0.1:%d/mcp?s=%s", port, token),
		SelfExe:      selfExe,
		Port:         port,
	}, nil
}
