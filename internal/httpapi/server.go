package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/apply"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/distill"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/mirror"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/retro"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/vault"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/workspace"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/wrapper"
)

type Deps struct {
	Store   *store.Store
	Mirror  *mirror.Mirror
	Bus     *bus.Bus
	Applier *apply.Applier
	MCP     http.Handler // mantém Deps.MCP como http.Handler (main.go passa mcpSvc.HTTPHandler()); NÃO alterar o tipo
	Wrapper   *wrapper.Manager
	Workspace *workspace.Manager  // workspace gerenciado por escopo (Task 4/7)
	Adapters  *adapter.Registry
	Port      int             // porta de escuta, p/ montar a URL MCP por sessão
	Vault     *vault.Vault
	Distiller *distill.Engine // motor de varredura (fase 4)
	Handoff   SummaryGeneratorIface // optional; nil = handoff indisponível
	Spawner   Spawner               // optional; nil = handoff indisponível
	Retro     *retro.Service        // fase 8: análise retroativa; nil = indisponível
}

type Server struct {
	deps Deps
	mux  *http.ServeMux
}

func New(deps Deps) *Server {
	s := &Server{deps: deps, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	if s.deps.MCP != nil {
		s.mux.Handle("/mcp", s.deps.MCP)
		s.mux.Handle("/mcp/", s.deps.MCP)
	}
	s.mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})
	s.routesProjects()
	s.routesFS()
	s.routesSuggestions()
	s.routesLineage()
	s.routesSkillPkg()
	s.routesSessions()
	s.routesModels()
	s.routesSecrets()
	s.routesSweep()
	s.routesHandoff()
	s.routesRetro()
	s.routesWS()
	s.routesStatic()
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func decode[T any](r *http.Request) (T, error) {
	var v T
	err := json.NewDecoder(r.Body).Decode(&v)
	return v, err
}
