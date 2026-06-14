package httpapi

import (
	"net/http"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func (s *Server) routesPipelines() {
	// GET /api/projects/:id/pipelines — lista skills compostas (kind=pipeline).
	s.mux.HandleFunc("GET /api/projects/{id}/pipelines", func(w http.ResponseWriter, r *http.Request) {
		pipelines, err := s.deps.Store.ListPipelines(r.PathValue("id"))
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, pipelines)
	})

	// POST /api/projects/:id/pipelines {name,steps[]} — cria pipeline.
	s.mux.HandleFunc("POST /api/projects/{id}/pipelines", func(w http.ResponseWriter, r *http.Request) {
		pid := r.PathValue("id")
		in, err := decode[struct {
			Name  string               `json:"name"`
			Steps []store.PipelineStep `json:"steps"`
		}](r)
		if err != nil || in.Name == "" {
			writeErr(w, 400, "name e steps obrigatórios")
			return
		}
		sk, err := s.deps.Store.CreatePipeline(pid, in.Name, in.Steps)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, 201, sk)
	})

	// PUT /api/pipelines/:skillId {name,steps[]} — atualiza pipeline.
	s.mux.HandleFunc("PUT /api/pipelines/{skillId}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("skillId")
		in, err := decode[struct {
			Name  string               `json:"name"`
			Steps []store.PipelineStep `json:"steps"`
		}](r)
		if err != nil || in.Name == "" {
			writeErr(w, 400, "name e steps obrigatórios")
			return
		}
		if err := s.deps.Store.UpdatePipeline(id, in.Name, in.Steps); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		sk, err := s.deps.Store.GetSkill(id)
		if err != nil {
			notFoundOr500(w, err, "pipeline não encontrada")
			return
		}
		writeJSON(w, 200, sk)
	})
}
