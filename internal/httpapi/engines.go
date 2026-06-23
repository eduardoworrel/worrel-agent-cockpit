package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func (s *Server) routesEngines() {
	s.mux.HandleFunc("GET /api/engines", func(w http.ResponseWriter, r *http.Request) {
		if s.deps.Engines == nil {
			writeErr(w, http.StatusServiceUnavailable, "motores indisponíveis")
			return
		}
		projectID := r.URL.Query().Get("project_id")
		type item struct {
			Spec   any               `json:"spec"`
			Config map[string]string `json:"config"`
		}
		out := []item{}
		for _, sp := range s.deps.Engines.List() {
			cfg, err := s.deps.Store.ResolveEngineConfig(sp.ID, projectID, s.deps.Engines.Defaults(sp.ID))
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			out = append(out, item{Spec: sp, Config: cfg})
		}
		writeJSON(w, http.StatusOK, out)
	})

	s.mux.HandleFunc("GET /api/engines/activity", func(w http.ResponseWriter, r *http.Request) {
		log, err := s.deps.Store.ListEngineLog(100)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, log)
	})

	s.mux.HandleFunc("GET /api/engines/{id}/backlog", func(w http.ResponseWriter, r *http.Request) {
		n, err := s.deps.Store.CountUnrunEndedSessions(r.PathValue("id"), r.URL.Query().Get("project_id"))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"unanalyzed": n})
	})

	s.mux.HandleFunc("GET /api/engines/{id}/enabled", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		sessionID := r.URL.Query().Get("session_id")
		// default por motor: summary OFF, interpret ON; query "default" sobrepõe.
		def := id == "interpret"
		if d := r.URL.Query().Get("default"); d != "" {
			def = d == "true"
		}
		writeJSON(w, http.StatusOK, map[string]bool{
			"enabled": s.deps.Store.EngineEnabled(id, sessionID, def),
		})
	})

	s.mux.HandleFunc("GET /api/engines/{id}/settings", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		sessionID := r.URL.Query().Get("session_id")
		get := func(key string) string {
			if sessionID != "" {
				if m, err := s.deps.Store.GetEngineConfig(id, "session:"+sessionID); err == nil {
					if v, ok := m[key]; ok && v != "" {
						return v
					}
				}
			}
			if m, err := s.deps.Store.GetEngineConfig(id, ""); err == nil {
				return m[key]
			}
			return ""
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": s.deps.Store.EngineEnabled(id, sessionID, id == "interpret"),
			"harness": get("harness"),
			"model":   get("model"),
		})
	})

	s.mux.HandleFunc("PUT /api/engines/{id}/config", func(w http.ResponseWriter, r *http.Request) {
		if s.deps.Engines == nil {
			writeErr(w, http.StatusServiceUnavailable, "motores indisponíveis")
			return
		}
		var body struct {
			ProjectID string `json:"project_id"`
			Key       string `json:"key"`
			Value     string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.deps.Store.SetEngineConfig(r.PathValue("id"), body.Key, body.Value, body.ProjectID); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	s.mux.HandleFunc("POST /api/engines/{id}/run", func(w http.ResponseWriter, r *http.Request) {
		if s.deps.Engines == nil {
			writeErr(w, http.StatusServiceUnavailable, "motores indisponíveis")
			return
		}
		var body struct {
			ProjectID string `json:"project_id"`
			SessionID string `json:"session_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.deps.Engines.Run(r.Context(), s.deps.Store, r.PathValue("id"), body.ProjectID, body.SessionID); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	s.mux.HandleFunc("POST /api/engines/{id}/reprocess", func(w http.ResponseWriter, r *http.Request) {
		if s.deps.Engines == nil {
			writeErr(w, http.StatusServiceUnavailable, "motores indisponíveis")
			return
		}
		id := r.PathValue("id")
		var body struct {
			ProjectID string `json:"project_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		sessions, err := s.deps.Store.UnrunEndedSessions(id, body.ProjectID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		// guarda: um lote por motor de cada vez.
		s.reprocMu.Lock()
		if s.reproc[id] {
			s.reprocMu.Unlock()
			writeErr(w, http.StatusConflict, "reprocessamento já em andamento")
			return
		}
		s.reproc[id] = true
		s.reprocMu.Unlock()

		go s.runReprocess(id, body.ProjectID, sessions)
		writeJSON(w, http.StatusAccepted, map[string]int{"total": len(sessions)})
	})
}

// runReprocess roda o motor sobre cada sessão não-analisada, marcando engine_runs
// no sucesso (idempotência) e publicando progresso no bus. Sessão sem erro marca
// mesmo rendendo 0 (não reprocessa vazio); erro real não marca (retentável).
func (s *Server) runReprocess(engineID, projectID string, sessions []*store.Session) {
	defer func() {
		s.reprocMu.Lock()
		delete(s.reproc, engineID)
		s.reprocMu.Unlock()
	}()
	total := len(sessions)
	processed, errors := 0, 0
	publish := func(typ string, payload map[string]any) {
		if s.deps.Bus != nil {
			s.deps.Bus.Publish(bus.Event{Type: typ, Payload: payload})
		}
	}
	for i, sess := range sessions {
		if err := s.deps.Engines.Run(context.Background(), s.deps.Store, engineID, sess.ProjectID, sess.ID); err != nil {
			errors++
		} else {
			_ = s.deps.Store.MarkEngineRun(engineID, sess.ID)
			processed++
		}
		publish("engine.reprocess.progress", map[string]any{"engine_id": engineID, "done": i + 1, "total": total})
	}
	publish("engine.reprocess.done", map[string]any{"engine_id": engineID, "processed": processed, "errors": errors})
}
