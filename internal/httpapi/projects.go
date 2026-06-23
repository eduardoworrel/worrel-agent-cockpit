package httpapi

import (
	"database/sql"
	"errors"
	"log"
	"net/http"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
)

// notFoundOr500 mapeia sql.ErrNoRows para 404 e o restante para 500.
func notFoundOr500(w http.ResponseWriter, err error, msg string) {
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, msg)
		return
	}
	writeErr(w, 500, err.Error())
}

func (s *Server) routesProjects() {
	s.mux.HandleFunc("GET /api/projects", func(w http.ResponseWriter, r *http.Request) {
		list, err := s.deps.Store.ListProjects()
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, list)
	})
	s.mux.HandleFunc("POST /api/projects", func(w http.ResponseWriter, r *http.Request) {
		in, err := decode[struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Dirs        []string `json:"dirs"`
			GitURL      string   `json:"git_url"`
		}](r)
		if err != nil || in.Name == "" {
			writeErr(w, 400, "name obrigatório")
			return
		}
		p, err := s.deps.Store.CreateProject(in.Name, in.Description)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		dirs := append([]string{}, in.Dirs...)
		// URL git: clona para <root>/repos/<slug> e adiciona o clone como pasta do projeto.
		if in.GitURL != "" {
			clonePath, cerr := s.deps.Workspace.CloneRepo(p.Slug, in.GitURL)
			if cerr != nil {
				_ = s.deps.Store.DeleteProject(p.ID)
				writeErr(w, 400, cerr.Error())
				return
			}
			dirs = append(dirs, clonePath)
		}
		for _, d := range dirs {
			if err := s.deps.Store.AddProjectDir(p.ID, d); err != nil {
				writeErr(w, 500, err.Error())
				return
			}
		}
		// Cria os symlinks do workspace já na criação (não só no spawn da sessão).
		if _, err := s.deps.Workspace.SyncProject(p.Slug, dirs); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		p, err = s.deps.Store.GetProject(p.ID)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 201, p)
	})
	s.mux.HandleFunc("GET /api/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.deps.Store.GetProject(r.PathValue("id"))
		if err != nil {
			notFoundOr500(w, err, "projeto não encontrado")
			return
		}
		writeJSON(w, 200, p)
	})
	s.mux.HandleFunc("PUT /api/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		in, err := decode[struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}](r)
		if err != nil || in.Name == "" {
			writeErr(w, 400, "name obrigatório")
			return
		}
		if err := s.deps.Store.UpdateProject(r.PathValue("id"), in.Name, in.Description); err != nil {
			notFoundOr500(w, err, "projeto não encontrado")
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	s.mux.HandleFunc("POST /api/projects/{id}/archive", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := s.deps.Store.ArchiveProject(id); err != nil {
			notFoundOr500(w, err, "projeto não encontrado")
			return
		}
		s.deps.Bus.Publish(bus.Event{Type: "project.archived", Payload: map[string]any{"id": id}})
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	s.mux.HandleFunc("GET /api/projects/{id}/memory", func(w http.ResponseWriter, r *http.Request) {
		m, err := s.deps.Store.GetMemory(r.PathValue("id"))
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, m)
	})
	s.mux.HandleFunc("PUT /api/projects/{id}/memory", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		in, err := decode[struct {
			Content string `json:"content"`
			Note    string `json:"note"`
		}](r)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		p, err := s.deps.Store.GetProject(id)
		if err != nil {
			notFoundOr500(w, err, "projeto não encontrado")
			return
		}
		v, err := s.deps.Store.SaveMemory(id, in.Content, in.Note)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if err := s.deps.Mirror.WriteMemory(p.Slug, in.Content); err != nil {
			log.Printf("mirror: %v", err)
		}
		writeJSON(w, 200, v)
	})
	s.mux.HandleFunc("GET /api/projects/{id}/memory/versions", func(w http.ResponseWriter, r *http.Request) {
		vs, err := s.deps.Store.ListMemoryVersions(r.PathValue("id"))
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, vs)
	})
	s.mux.HandleFunc("POST /api/projects/{id}/memory/revert", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		in, err := decode[struct {
			VersionID int64 `json:"version_id"`
		}](r)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		v, err := s.deps.Store.RevertMemory(id, in.VersionID)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if p, err := s.deps.Store.GetProject(id); err == nil {
			if err := s.deps.Mirror.WriteMemory(p.Slug, v.Content); err != nil {
				log.Printf("mirror: %v", err)
			}
		} else {
			log.Printf("mirror: %v", err)
		}
		writeJSON(w, 200, v)
	})
	s.mux.HandleFunc("GET /api/projects/{id}/memory/entries", func(w http.ResponseWriter, r *http.Request) {
		entries, err := s.deps.Store.ListMemoryEntries(r.PathValue("id"), false)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, entries)
	})
	s.mux.HandleFunc("DELETE /api/projects/{id}/memory/entries/{eid}", func(w http.ResponseWriter, r *http.Request) {
		if err := s.deps.Store.DeleteMemoryEntry(r.PathValue("eid")); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})
	s.mux.HandleFunc("GET /api/skills", func(w http.ResponseWriter, r *http.Request) {
		list, err := s.deps.Store.ListSkills(r.URL.Query().Get("project_id"))
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, list)
	})
	s.mux.HandleFunc("POST /api/projects/{id}/skills", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		in, err := decode[struct {
			Name    string `json:"name"`
			Content string `json:"content"`
		}](r)
		if err != nil || in.Name == "" {
			writeErr(w, 400, "name obrigatório")
			return
		}
		p, err := s.deps.Store.GetProject(id)
		if err != nil {
			notFoundOr500(w, err, "projeto não encontrado")
			return
		}
		sk, err := s.deps.Store.CreateSkill(id, in.Name, in.Content)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if err := s.deps.Mirror.WriteSkill(p.Slug, sk.Slug, sk.Content); err != nil {
			log.Printf("mirror: %v", err)
		}
		writeJSON(w, 201, sk)
	})
	s.mux.HandleFunc("PUT /api/skills/{id}", func(w http.ResponseWriter, r *http.Request) {
		in, err := decode[struct {
			Name    string `json:"name"`
			Content string `json:"content"`
		}](r)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		id := r.PathValue("id")
		if err := s.deps.Store.UpdateSkill(id, in.Name, in.Content); err != nil {
			notFoundOr500(w, err, "skill não encontrada")
			return
		}
		sk, err := s.deps.Store.GetSkill(id)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if p, err := s.deps.Store.GetProject(sk.ProjectID); err == nil {
			if err := s.deps.Mirror.WriteSkill(p.Slug, sk.Slug, sk.Content); err != nil {
				log.Printf("mirror: %v", err)
			}
		} else {
			log.Printf("mirror: %v", err)
		}
		writeJSON(w, 200, sk)
	})
	s.mux.HandleFunc("GET /api/sessions", func(w http.ResponseWriter, r *http.Request) {
		list, err := s.deps.Store.ListSessions(r.URL.Query().Get("project_id"))
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, list)
	})
	s.mux.HandleFunc("GET /api/projects/{id}/agents", func(w http.ResponseWriter, r *http.Request) {
		list, err := s.deps.Store.ListAgents(r.PathValue("id"))
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, list)
	})
	s.mux.HandleFunc("GET /api/projects/{id}/skill-candidates", func(w http.ResponseWriter, r *http.Request) {
		list, err := s.deps.Store.ListSkillCandidates(r.PathValue("id"), "")
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, list)
	})
	s.mux.HandleFunc("GET /api/settings", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{
			"retention_days":   s.deps.Store.GetSetting("retention_days", "30"),
			"headless_adapter": s.deps.Store.GetSetting("headless_adapter", "claude-code"),
		})
	})
	s.mux.HandleFunc("PUT /api/settings", func(w http.ResponseWriter, r *http.Request) {
		in, err := decode[map[string]string](r)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		for k, v := range in {
			if err := s.deps.Store.SetSetting(k, v); err != nil {
				writeErr(w, 500, err.Error())
				return
			}
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
}
