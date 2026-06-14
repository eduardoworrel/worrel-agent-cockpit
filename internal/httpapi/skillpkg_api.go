package httpapi

import (
	"net/http"
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/skillpkg"
)

func firstLine(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(strings.TrimLeft(ln, "#"))
		ln = strings.TrimSpace(ln)
		if ln != "" {
			return ln
		}
	}
	return ""
}

func (s *Server) routesSkillPkg() {
	// GET /api/skills/:id/export → retorna SKILL.md (ou escreve em ?dir=).
	s.mux.HandleFunc("GET /api/skills/{id}/export", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		sk, err := s.deps.Store.GetSkill(id)
		if err != nil {
			notFoundOr500(w, err, "skill não encontrada")
			return
		}
		// description (obrigatório no padrão aberto, spec §8.1): primeira linha
		// significativa do conteúdo, ou o nome como fallback.
		desc := firstLine(sk.Content)
		if desc == "" {
			desc = sk.Name
		}
		gens, _ := s.deps.Store.ListGenerations(sk.ID)
		lineage := make([]skillpkg.GenSummary, 0, len(gens))
		for _, g := range gens {
			lineage = append(lineage, skillpkg.GenSummary{
				Generation: g.Generation, EvolutionType: g.EvolutionType,
				ChangeSummary: g.ChangeSummary, Authorship: g.Authorship,
			})
		}
		pkg := &skillpkg.Package{
			Meta: skillpkg.Meta{
				Name:        sk.Name,
				Description: desc,
				Origin:      sk.Origin,
			},
			Content: sk.Content,
			Sidecar: &skillpkg.Sidecar{
				SkillID:          sk.ID,
				Origin:           sk.Origin,
				ActiveGeneration: sk.ActiveGeneration,
				Generations:      len(gens),
				Lineage:          lineage,
			},
		}
		// Se um diretório foi pedido, escreve SKILL.md + cockpit.meta.json lá.
		if dir := r.URL.Query().Get("dir"); dir != "" {
			if err := skillpkg.WriteDir(dir, sk.Slug, pkg); err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			writeJSON(w, 200, map[string]string{"dir": dir + "/" + sk.Slug})
			return
		}
		rendered := skillpkg.Render(pkg)
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="SKILL.md"`)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(rendered))
	})

	// POST /api/projects/:id/skills/import → importa SKILL.md
	s.mux.HandleFunc("POST /api/projects/{id}/skills/import", func(w http.ResponseWriter, r *http.Request) {
		projectID := r.PathValue("id")
		in, err := decode[struct {
			Content string `json:"content"`
		}](r)
		if err != nil || in.Content == "" {
			writeErr(w, 400, "content obrigatório")
			return
		}
		pkg, err := skillpkg.Parse(in.Content)
		if err != nil {
			writeErr(w, 400, "SKILL.md inválido: "+err.Error())
			return
		}
		name := pkg.Meta.Name
		if name == "" {
			name = "Skill importada"
		}
		origin := pkg.Meta.Origin
		if origin == "" {
			origin = "imported"
		}
		sk, err := s.deps.Store.CreateSkillWithOrigin(projectID, name, pkg.Content, origin, "manual")
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 201, sk)
	})
}
