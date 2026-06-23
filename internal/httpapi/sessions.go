package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/wrapper"
)

// termWriteTimeout é o deadline de cada escrita no WS do terminal; estourado,
// só ESTE assinante é derrubado (o PTY e os demais assinantes seguem).
const termWriteTimeout = 10 * time.Second

// termReadLimit limita o tamanho de mensagens client→server no WS do terminal.
const termReadLimit = 64 << 10 // 64KB

func (s *Server) routesSessions() {
	s.mux.HandleFunc("GET /api/adapters", s.handleAdapters)
	// GET /api/sessions already registered in projects.go (routesProjects)
	s.mux.HandleFunc("POST /api/projects/{id}/sessions", s.handleCreateSession)
	s.mux.HandleFunc("POST /api/sessions/{id}/kill", s.handleKillSession)
	s.mux.HandleFunc("GET /api/sessions/{id}/term", s.handleTerm)
	s.mux.HandleFunc("POST /api/sessions/{id}/paste-image", s.handlePasteImage)
	s.mux.HandleFunc("POST /api/sessions", s.handleCreateFreeSession)
	s.mux.HandleFunc("POST /api/sessions/{id}/classify", s.handleClassifySession)
	s.mux.HandleFunc("POST /api/sessions/{id}/promote", s.handlePromoteSession)
	s.mux.HandleFunc("POST /api/sessions/{id}/archive", s.handleArchiveSession)
	s.mux.HandleFunc("GET /api/sessions/active", s.handleActiveSessions)
}

// handleArchiveSession marca a sessão como arquivada: ela some da listagem
// padrão (histórico) sem ser apagada — transcript, sugestões e auditoria
// permanecem. Idempotente em relação ao status.
func (s *Server) handleArchiveSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.deps.Store.ArchiveSession(id); err != nil {
		notFoundOr500(w, err, "sessão não encontrada")
		return
	}
	s.deps.Bus.Publish(bus.Event{Type: "session.archived", Payload: map[string]any{"id": id}})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handleAdapters(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.deps.Adapters.Detected())
}

// spawnFor monta opts/spec e spawna; usado pela sessão de projeto e pela livre.
func (s *Server) spawnFor(w http.ResponseWriter, sess *store.Session, adapterID, skill, persona string) {
	ad, ok := s.deps.Adapters.Get(adapterID)
	if !ok {
		writeErr(w, 400, "adaptador desconhecido: "+adapterID)
		return
	}
	opts, err := wrapper.BuildSpawnOpts(s.deps.Store, s.deps.Workspace, sess.ID, s.deps.Port, skill, persona)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// persiste o cwd resolvido na sessão (faixa de abas / hub mostram)
	_ = s.deps.Store.SetSessionWorkspaceDir(sess.ID, opts.WorkingDir)
	spec, err := ad.BuildInteractive(opts)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// SpawnWithAdapter liga o tracker de contexto (session.context) à
	// sessão. Para claude-code o external ref é o
	// próprio sess.ID (BuildInteractive passa --session-id sess.ID).
	ref := adapter.SessionRef{Adapter: adapterID, ExternalRef: sess.ID}
	if err := s.deps.Wrapper.SpawnWithAdapter(sess.ID, spec, ad, ref); err != nil {
		_ = s.deps.Store.EndSession(sess.ID)
		writeErr(w, 500, err.Error())
		return
	}
	s.deps.Bus.Publish(bus.Event{Type: "session.started", Payload: map[string]any{"id": sess.ID, "project_id": sess.ProjectID}})
	fresh, _ := s.deps.Store.GetSession(sess.ID)
	writeJSON(w, 201, fresh)
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	var body struct {
		Adapter string `json:"adapter"`
		Skill   string `json:"skill"`    // conteúdo opcional p/ "iniciar a partir de skill"
		SkillID string `json:"skill_id"` // id de skill a resolver no backend
		AgentID string `json:"agent_id"` // id de agente; persona vai para SystemAppend
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "body inválido: "+err.Error())
		return
	}
	if _, ok := s.deps.Adapters.Get(body.Adapter); !ok {
		writeErr(w, 400, "adaptador desconhecido: "+body.Adapter)
		return
	}
	if body.SkillID != "" && body.Skill == "" {
		if sk, err := s.deps.Store.GetSkill(body.SkillID); err == nil {
			body.Skill = sk.Content
		}
	}
	persona := ""
	if body.AgentID != "" {
		if ag, err := s.deps.Store.GetAgent(body.AgentID); err == nil {
			persona = ag.Persona
		}
	}
	sess, err := s.deps.Store.CreateSession(&store.Session{
		ProjectID: projectID, Adapter: body.Adapter, Mode: "wrapper",
	})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.spawnFor(w, sess, body.Adapter, body.Skill, persona)
}

func (s *Server) handleCreateFreeSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Adapter string   `json:"adapter"`
		Skill   string   `json:"skill"`
		Dirs    []string `json:"dirs"` // pastas opcionais a linkar no scratch
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "body inválido")
		return
	}
	if _, ok := s.deps.Adapters.Get(body.Adapter); !ok {
		writeErr(w, 400, "adaptador desconhecido: "+body.Adapter)
		return
	}
	sess, err := s.deps.Store.CreateSession(&store.Session{Adapter: body.Adapter, Mode: "wrapper"})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	scratch, err := s.deps.Workspace.ScratchWorkspace(sess.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if len(body.Dirs) > 0 {
		_ = s.deps.Workspace.SyncSymlinks(scratch, body.Dirs)
	}
	_ = s.deps.Store.SetSessionWorkspaceDir(sess.ID, scratch)
	sess, _ = s.deps.Store.GetSession(sess.ID)
	s.spawnFor(w, sess, body.Adapter, body.Skill, "")
}

func (s *Server) handleClassifySession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ProjectID == "" {
		writeErr(w, 400, "project_id obrigatório")
		return
	}
	if err := s.deps.Store.ClassifySession(id, body.ProjectID); err != nil {
		notFoundOr500(w, err, "sessão não encontrada")
		return
	}
	s.deps.Bus.Publish(bus.Event{Type: "session.classified", Payload: map[string]any{"id": id, "project_id": body.ProjectID}})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handlePromoteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeErr(w, 400, "name obrigatório")
		return
	}
	p, err := s.deps.Store.PromoteSessionToProject(id, body.Name, body.Description)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.deps.Bus.Publish(bus.Event{Type: "session.classified", Payload: map[string]any{"id": id, "project_id": p.ID}})
	writeJSON(w, 201, p)
}

func (s *Server) handleActiveSessions(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Store.ListActiveWrapperSessions()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) handleKillSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// Sessão do motor: encerra o processo stream-json.
	if s.deps.Engine != nil && s.deps.Engine.Has(id) {
		s.deps.Engine.Close(id)
	}
	// Best-effort: mata o PTY se ainda estiver vivo NESTE processo. Após um
	// restart do servidor o processo não existe mais no mapa em memória — isso
	// não é erro (a sessão fica "órfã": active no banco, sem PTY vivo).
	_ = s.deps.Wrapper.Kill(id)
	// Encerra a sessão no store SEMPRE, para que ela saia da faixa de ativas
	// mesmo quando o PTY já não existe. Sem isso, o × não disparava nada para
	// sessões órfãs (Kill falhava com 404 e a sessão seguia active).
	if err := s.deps.Store.EndSession(id); err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	w.WriteHeader(204)
}

func (s *Server) handleTerm(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if !s.deps.Wrapper.IsRunning(sessionID) {
		writeErr(w, 404, "sessão não está rodando")
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(termReadLimit)

	// server -> client: bytes de PTY como mensagens binárias.
	// fn é chamado pela goroutine de escrita dedicada deste assinante
	// (criada por Subscribe) — escritas lentas nunca bloqueiam o read loop
	// do PTY. No deadline/erro, derruba só esta conexão: Close faz o
	// ReadMessage abaixo retornar e o defer unsub limpar o assinante.
	unsub := s.deps.Wrapper.Subscribe(sessionID, func(p []byte) {
		_ = conn.SetWriteDeadline(time.Now().Add(termWriteTimeout))
		if err := conn.WriteMessage(websocket.BinaryMessage, p); err != nil {
			_ = conn.Close()
		}
	})
	defer unsub()

	// client -> server: JSON {type:"stdin"|"resize", ...}
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var m struct {
			Type string `json:"type"`
			Data string `json:"data"`
			Cols uint16 `json:"cols"`
			Rows uint16 `json:"rows"`
		}
		if json.Unmarshal(msg, &m) != nil {
			continue
		}
		switch m.Type {
		case "stdin":
			_ = s.deps.Wrapper.Write(sessionID, []byte(m.Data))
		case "resize":
			_ = s.deps.Wrapper.Resize(sessionID, m.Cols, m.Rows)
		}
	}
}
