package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/apply"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func (s *Server) routesSuggestions() {
	s.mux.HandleFunc("GET /api/suggestions", func(w http.ResponseWriter, r *http.Request) {
		list, err := s.deps.Store.ListSuggestions(
			r.URL.Query().Get("project_id"), r.URL.Query().Get("status"))
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, list)
	})
	s.mux.HandleFunc("POST /api/suggestions", func(w http.ResponseWriter, r *http.Request) {
		in, err := decode[store.Suggestion](r)
		if err != nil || in.Type == "" || in.Title == "" {
			writeErr(w, 400, "type e title obrigatórios")
			return
		}
		sg, err := s.deps.Store.CreateSuggestion(&in)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		s.deps.Bus.Publish(bus.Event{Type: "suggestion.created", Payload: sg})
		writeJSON(w, 201, sg)
	})
	s.mux.HandleFunc("POST /api/suggestions/{id}/accept", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		// optional edit before accept
		in, err := decode[struct {
			Title   string `json:"title"`
			Payload string `json:"payload"`
		}](r)
		if err == nil && in.Payload != "" {
			if err := s.deps.Store.UpdateSuggestionPayload(id, in.Title, in.Payload); err != nil {
				notFoundOr500(w, err, "sugestão não encontrada")
				return
			}
		}
		if old := r.URL.Query().Get("supersede"); old != "" {
			if err := s.deps.Applier.AcceptSuperseding(id, old); err != nil {
				writeAcceptErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			return
		}
		if as := r.URL.Query().Get("as"); as != "" {
			if err := s.deps.Applier.AcceptAs(id, as); err != nil {
				writeAcceptErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			return
		}
		if err := s.deps.Applier.Accept(id); err != nil {
			writeAcceptErr(w, err)
			return
		}
		sg, err := s.deps.Store.GetSuggestion(id)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		s.deps.Bus.Publish(bus.Event{Type: "suggestion.resolved", Payload: sg})
		writeJSON(w, 200, sg)
	})
	s.mux.HandleFunc("POST /api/suggestions/{id}/reject", s.resolveHandler("rejected"))
	s.mux.HandleFunc("POST /api/suggestions/{id}/defer", s.resolveHandler("deferred"))
	s.mux.HandleFunc("PUT /api/suggestions/{id}/type", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		in, err := decode[struct {
			Type string `json:"type"`
		}](r)
		if err != nil || in.Type == "" {
			writeErr(w, 400, "type obrigatório")
			return
		}
		if err := s.deps.Store.ReclassifySuggestion(id, in.Type); err != nil {
			notFoundOr500(w, err, "sugestão não encontrada ou já resolvida")
			return
		}
		sg, err := s.deps.Store.GetSuggestion(id)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, sg)
	})
}

func (s *Server) resolveHandler(status string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		// Critério 9: rejeitar um segredo detectado o suprime por hash para que
		// varreduras retroativas futuras não voltem a sugeri-lo.
		if status == "rejected" {
			if sg, err := s.deps.Store.GetSuggestion(id); err == nil && sg.Type == "secret.detected" {
				var pl struct {
					Hash string `json:"hash"`
				}
				if json.Unmarshal([]byte(sg.Payload), &pl) == nil && pl.Hash != "" {
					_ = s.deps.Store.SuppressSecret(pl.Hash)
				}
			}
		}
		if err := s.deps.Store.ResolveSuggestion(id, status); err != nil {
			notFoundOr500(w, err, "sugestão não encontrada")
			return
		}
		sg, err := s.deps.Store.GetSuggestion(id)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		s.deps.Bus.Publish(bus.Event{Type: "suggestion.resolved", Payload: sg})
		writeJSON(w, 200, sg)
	}
}

// writeAcceptErr mapeia erros de aceitar sugestão para o status HTTP certo:
// sugestão inexistente → 404; já resolvida → 409; demais → 500. Usado por
// todos os caminhos de accept (genérico, ?supersede=, ?as=).
func writeAcceptErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, 404, "sugestão não encontrada")
	case errors.Is(err, apply.ErrAlreadyResolved):
		writeErr(w, 409, err.Error())
	default:
		writeErr(w, 500, err.Error())
	}
}
