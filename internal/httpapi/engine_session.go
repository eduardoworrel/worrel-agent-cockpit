package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/streamengine"
)

func (s *Server) routesEngineSessions() {
	s.mux.HandleFunc("POST /api/sessions/engine", s.handleCreateEngineSession)
}

// handleCreateEngineSession cria uma sessão dirigida pelo MOTOR (stream-json):
// sem PTY, sem hook, sem ask. A Home a vê como uma sessão viva e interage por
// ela via o canal AG-UI (snapshot/respond/prompt).
func (s *Server) handleCreateEngineSession(w http.ResponseWriter, r *http.Request) {
	if s.deps.Engine == nil {
		writeErr(w, 503, "motor indisponível")
		return
	}
	in, _ := decode[struct {
		ProjectID string `json:"project_id"`
		Mode      string `json:"mode"`   // modo de permissão ("" = auto)
		Memory    string `json:"memory"` // "inicio" injeta a memória; "consulta" liga o MCP
	}](r)

	sess, err := s.deps.Store.CreateSession(&store.Session{
		ProjectID: in.ProjectID,
		Adapter:   "engine", // marca: dirigida pelo motor stream-json
		Mode:      "wrapper", // entra na faixa de sessões vivas da Home
	})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// cwd: com projeto, usa o workspace gerenciado do escopo (symlinks p/ as
	// pastas reais, incluindo o repo clonado); sem projeto, um scratch efêmero.
	// Em ambos é headless (stream-json) → sem trust prompt.
	var cwd string
	if in.ProjectID != "" {
		proj, perr := s.deps.Store.GetProject(in.ProjectID)
		if perr != nil {
			_ = s.deps.Store.EndSession(sess.ID)
			notFoundOr500(w, perr, "projeto não encontrado")
			return
		}
		cwd, err = s.deps.Workspace.SyncProject(proj.Slug, proj.Dirs)
	} else {
		cwd, err = s.deps.Workspace.ScratchWorkspace(sess.ID)
	}
	if err != nil {
		_ = s.deps.Store.EndSession(sess.ID)
		writeErr(w, 500, err.Error())
		return
	}
	_ = s.deps.Store.SetSessionWorkspaceDir(sess.ID, cwd)

	// Memória do projeto: "inicio" injeta o markdown no system prompt; "consulta"
	// liga o MCP do worrel para o agente buscar a memória (get_memory) sob demanda.
	opts := streamengine.Opts{Mode: in.Mode}
	if in.ProjectID != "" {
		switch in.Memory {
		case "inicio":
			mem := projectMemoryText(s.deps.Store, in.ProjectID)
			if mem != "" {
				opts.SystemAppend = "# Memória do projeto (carregada no início desta sessão)\n\n" + mem
			}
		case "consulta":
			token := uuid.NewString()
			if s.deps.Store.SetSessionMCPToken(sess.ID, token) == nil {
				opts.MCPURL = fmt.Sprintf("http://127.0.0.1:%d/mcp?s=%s", s.deps.Port, token)
			}
		}
	}

	// temporário: provider fixo até a Task 3 ligar o request.
	if err := s.deps.Engine.Start(context.Background(), "claude-code", sess.ID, cwd, opts); err != nil {
		_ = s.deps.Store.EndSession(sess.ID)
		writeErr(w, 500, err.Error())
		return
	}
	s.deps.Bus.Publish(bus.Event{Type: "session.started", Payload: map[string]any{"id": sess.ID, "project_id": sess.ProjectID}})
	fresh, _ := s.deps.Store.GetSession(sess.ID)
	writeJSON(w, 201, fresh)
}

// projectMemoryText reúne a memória do projeto para injetar: a versão editável
// (MEMORY.md) + os fatos destilados (entries). Vazio se não houver nada.
func projectMemoryText(st *store.Store, projectID string) string {
	var parts []string
	if v, err := st.GetMemory(projectID); err == nil && v != nil {
		if c := strings.TrimSpace(v.Content); c != "" {
			parts = append(parts, c)
		}
	}
	if r, err := st.RenderMemory(projectID); err == nil {
		if c := strings.TrimSpace(r); c != "" {
			parts = append(parts, c)
		}
	}
	return strings.Join(parts, "\n\n")
}
