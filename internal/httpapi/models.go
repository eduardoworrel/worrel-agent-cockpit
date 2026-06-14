package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
)

// modelsResponse é o shape do endpoint GET /api/adapters/{id}/models.
type modelsResponse struct {
	Models []string `json:"models"`
}

// routesModels registra GET /api/adapters/{id}/models.
//
// Contrato:
//   - 200 {"models":[...]} quando o adapter existe. A lista vem de ListModels()
//     se o adapter implementa adapter.ModelLister; caso contrário, {"models":[]}.
//   - 404 quando o id não existe no registry.
//   - 502 quando o adapter implementa ModelLister mas ListModels falha
//     (ex.: CLI não instalado) — o corpo ainda traz {"models":[]} para a UI
//     degradar para campo de texto livre.
//
// Usa um timeout de 10s no contexto para não pendurar a request num CLI lento.
func (s *Server) routesModels() {
	s.mux.HandleFunc("GET /api/adapters/{id}/models", func(w http.ResponseWriter, r *http.Request) {
		if s.deps.Adapters == nil {
			writeJSON(w, 200, modelsResponse{Models: []string{}})
			return
		}
		a, ok := s.deps.Adapters.Get(r.PathValue("id"))
		if !ok {
			writeErr(w, 404, "adapter não encontrado")
			return
		}
		lister, ok := a.(adapter.ModelLister)
		if !ok {
			writeJSON(w, 200, modelsResponse{Models: []string{}})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		models, err := lister.ListModels(ctx)
		if models == nil {
			models = []string{}
		}
		if err != nil {
			// Degradação graciosa: corpo vazio + status indicando falha do CLI.
			writeJSON(w, 502, modelsResponse{Models: models})
			return
		}
		writeJSON(w, 200, modelsResponse{Models: models})
	})
}
