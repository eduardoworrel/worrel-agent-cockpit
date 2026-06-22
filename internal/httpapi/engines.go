package httpapi

import (
	"encoding/json"
	"net/http"
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
}
