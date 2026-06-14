package httpapi

import (
	"fmt"
	"log"
	"strings"
)

// appendRecipeToMemory acrescenta a instrução de obtenção do segredo à memória
// do projeto, para que o agente encontre o contexto de onde buscar o valor.
// Falhas são logadas mas não propagadas (operação best-effort).
func (s *Server) appendRecipeToMemory(projectID, secretName, recipe string) {
	proj, err := s.deps.Store.GetProject(projectID)
	if err != nil {
		log.Printf("secrets: projeto %s não encontrado para espelhar receita: %v", projectID, err)
		return
	}

	cur, _ := s.deps.Store.GetMemory(projectID)
	var existing string
	if cur != nil {
		existing = cur.Content
	}

	marker := fmt.Sprintf("<!-- secret:%s -->", secretName)
	if strings.Contains(existing, marker) {
		// já registrado; não duplica
		return
	}

	entry := fmt.Sprintf("\n%s\n## Segredo: %s\n\n> Receita: %s\n", marker, secretName, recipe)
	updated := existing + entry

	if _, err := s.deps.Store.SaveMemory(projectID, updated, "receita do segredo "+secretName); err != nil {
		log.Printf("secrets: falha ao salvar memória do projeto %s: %v", projectID, err)
		return
	}

	if s.deps.Mirror != nil {
		if err := s.deps.Mirror.WriteMemory(proj.Slug, updated); err != nil {
			log.Printf("secrets: falha ao espelhar memória do projeto %s: %v", proj.Slug, err)
		}
	}
}
