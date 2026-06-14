package httpapi

import (
	"net/http"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func (s *Server) routesLineage() {
	// GET /api/skills/:id/generations — cadeia própria de gerações.
	s.mux.HandleFunc("GET /api/skills/{id}/generations", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		gens, err := s.deps.Store.ListGenerations(id)
		if err != nil {
			notFoundOr500(w, err, "skill não encontrada")
			return
		}
		writeJSON(w, 200, gens)
	})

	// GET /api/skills/:id/branches — ramificações variante (skills com esta como mãe).
	s.mux.HandleFunc("GET /api/skills/{id}/branches", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		branches, err := s.deps.Store.GenerationsWithParent(id)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, branches)
	})

	// POST /api/skills/:id/revert — reativa geração anterior (pointer flip).
	// POST /api/skills/:id/promote — promover ramificação = ativar geração (alias).
	revert := func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		in, err := decode[struct {
			Generation int64 `json:"generation"`
		}](r)
		if err != nil || in.Generation == 0 {
			writeErr(w, 400, "generation obrigatório")
			return
		}
		if err := s.deps.Store.ActivateGeneration(id, in.Generation); err != nil {
			notFoundOr500(w, err, "skill ou geração não encontrada")
			return
		}
		sk, err := s.deps.Store.GetSkill(id)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		// Atualiza o espelho em arquivos para a geração reativada.
		if proj, perr := s.deps.Store.GetProject(sk.ProjectID); perr == nil && s.deps.Mirror != nil {
			_ = s.deps.Mirror.WriteSkill(proj.Slug, sk.Slug, sk.Content)
		}
		if s.deps.Bus != nil {
			s.deps.Bus.Publish(bus.Event{Type: "skill.reverted",
				Payload: map[string]any{"skill_id": id, "generation": in.Generation}})
		}
		writeJSON(w, 200, sk)
	}
	s.mux.HandleFunc("POST /api/skills/{id}/revert", revert)
	s.mux.HandleFunc("POST /api/skills/{id}/promote", revert)

	// GET /api/skills/stats?project_id= — mapa skill_id→stats (colunas de saúde).
	s.mux.HandleFunc("GET /api/skills/stats", func(w http.ResponseWriter, r *http.Request) {
		skills, err := s.deps.Store.ListSkills(r.URL.Query().Get("project_id"))
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		out := map[string]*store.SkillStats{}
		for _, sk := range skills {
			st, err := s.deps.Store.SkillStats(sk.ID)
			if err != nil {
				continue
			}
			out[sk.ID] = st
		}
		writeJSON(w, 200, out)
	})

	// PUT /api/projects/:id/skills/policy — política em lote por projeto.
	s.mux.HandleFunc("PUT /api/projects/{id}/skills/policy", func(w http.ResponseWriter, r *http.Request) {
		pid := r.PathValue("id")
		in, err := decode[struct {
			Policy string `json:"policy"`
		}](r)
		if err != nil || in.Policy == "" {
			writeErr(w, 400, "policy obrigatório")
			return
		}
		if err := s.deps.Store.SetProjectSkillsPolicy(pid, in.Policy); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"policy": in.Policy})
	})

	// GET /api/skills/:id/stats
	s.mux.HandleFunc("GET /api/skills/{id}/stats", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		stats, err := s.deps.Store.SkillStats(id)
		if err != nil {
			notFoundOr500(w, err, "skill não encontrada")
			return
		}
		writeJSON(w, 200, stats)
	})

	// PUT /api/skills/:id/policy
	s.mux.HandleFunc("PUT /api/skills/{id}/policy", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		in, err := decode[struct {
			Policy string `json:"policy"`
		}](r)
		if err != nil || in.Policy == "" {
			writeErr(w, 400, "policy obrigatório")
			return
		}
		if err := s.deps.Store.SetSkillPolicy(id, in.Policy); err != nil {
			notFoundOr500(w, err, "skill não encontrada")
			return
		}
		sk, err := s.deps.Store.GetSkill(id)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, sk)
	})

}
