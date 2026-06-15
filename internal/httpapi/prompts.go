package httpapi

import (
	"net/http"
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/prompts"
)

// promptDTO descreve um prompt de análise: o texto efetivo (override ou default),
// o default embarcado e se há override salvo.
type promptDTO struct {
	Name       string `json:"name"`
	Value      string `json:"value"`
	Default    string `json:"default"`
	Overridden bool   `json:"overridden"`
}

func (s *Server) routesPrompts() {
	// Lista os prompts editáveis das análises (memory, skill, scope).
	s.mux.HandleFunc("GET /api/prompts", func(w http.ResponseWriter, r *http.Request) {
		out := make([]promptDTO, 0, len(prompts.Names))
		for _, name := range prompts.Names {
			def := prompts.Default(name)
			val := s.deps.Store.Prompt(name)
			out = append(out, promptDTO{
				Name:       name,
				Value:      val,
				Default:    def,
				Overridden: strings.TrimSpace(val) != strings.TrimSpace(def),
			})
		}
		writeJSON(w, 200, out)
	})

	// Salva (ou reseta) o override de um prompt. value vazio = volta ao default.
	s.mux.HandleFunc("PUT /api/prompts/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if prompts.Default(name) == "" {
			writeErr(w, 404, "prompt desconhecido")
			return
		}
		in, err := decode[struct {
			Value string `json:"value"`
		}](r)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		// value vazio (ou igual ao default) = remove o override.
		key := "prompt." + name
		if strings.TrimSpace(in.Value) == "" || strings.TrimSpace(in.Value) == strings.TrimSpace(prompts.Default(name)) {
			if err := s.deps.Store.SetSetting(key, ""); err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			writeJSON(w, 200, map[string]bool{"reset": true})
			return
		}
		if err := s.deps.Store.SetSetting(key, in.Value); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
}
