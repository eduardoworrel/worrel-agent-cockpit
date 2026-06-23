package httpapi

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/apply"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/ask"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/engine"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/mirror"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/streamengine"
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
	Ask       *ask.Broker           // pedidos de confirmação/escolha (balões); nil = indisponível
	Engines *engine.Registry // framework de motores (SP1); nil = indisponível
	Summarizer HeadlessLLM    // LLM headless p/ resumo de progresso da Home; nil = indisponível
	Engine *streamengine.Manager // motor stream-json (sessões dirigidas pela Home); nil = indisponível
}

type Server struct {
	deps      Deps
	mux       *http.ServeMux
	progress  *progressCache  // cache do resumo por IA por sessão (canal AG-UI/Home)
	titles    *progressCache  // cache do título "vivo" das sessões do motor
	interpret *interpretCache // cache da interpretação de turnos-fala (auto-mode)

	reprocMu sync.Mutex      // protege reproc
	reproc   map[string]bool // engineID em reprocessamento (impede lote concorrente)
}

func New(deps Deps) *Server {
	s := &Server{deps: deps, mux: http.NewServeMux(), progress: newProgressCache(), titles: newProgressCache(), interpret: newInterpretCache(), reproc: map[string]bool{}}
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
	s.routesEngines()
	s.routesLineage()
	s.routesSkillPkg()
	s.routesSessions()
	s.routesEngineSessions()
	s.routesInteraction()
	s.routesModels()
	s.routesSecrets()
	s.routesPipelines()
	s.routesAsks()
	s.routesReset()
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
