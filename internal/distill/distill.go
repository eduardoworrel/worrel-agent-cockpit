package distill

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
)

// sessionTranscript agrupa os eventos de uma sessão para o motor de varredura.
type sessionTranscript struct {
	SessionID string
	ProjectID string
	Events    []adapter.TranscriptEvent
}

type Candidate struct {
	Type           string   `json:"type"`
	Title          string   `json:"title"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Content        string   `json:"content"`
	SkillID        string   `json:"skill_id"`
	ParentSkillIDs []string `json:"parent_skill_ids"`
	Evidence       string   `json:"evidence"`
	ProjectID      string   `json:"project_id"`
}

var validTypes = map[string]bool{
	"skill.learned": true, "skill.correction": true, "skill.variant": true,
	"create_project": true, "add_memory": true,
}

func (c Candidate) valid() bool {
	if !validTypes[c.Type] || strings.TrimSpace(c.Title) == "" || strings.TrimSpace(c.Evidence) == "" {
		return false
	}
	switch c.Type {
	case "skill.learned", "skill.variant":
		return strings.TrimSpace(c.Name) != "" && strings.TrimSpace(c.Content) != ""
	case "skill.correction":
		return strings.TrimSpace(c.SkillID) != "" && strings.TrimSpace(c.Content) != ""
	case "create_project":
		return strings.TrimSpace(c.Description) != ""
	case "add_memory":
		// Memória por projeto (convenção/decisão/correção recorrente). O applier
		// lê o conteúdo do payload; aceitamos content OU description como fonte.
		return strings.TrimSpace(c.Content) != "" || strings.TrimSpace(c.Description) != ""
	}
	return false
}

// ParseCandidates extrai o array JSON (tolera cerca ```json), valida item a item.
// Retorna candidatos válidos e a contagem de descartados.
func ParseCandidates(raw string) (valid []Candidate, dropped int) {
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, "["); i >= 0 {
		if j := strings.LastIndex(s, "]"); j > i {
			s = s[i : j+1]
		}
	}
	var items []Candidate
	if err := json.Unmarshal([]byte(s), &items); err != nil {
		log.Printf("distill: JSON inválido: %v", err)
		return nil, 0
	}
	for _, it := range items {
		if it.valid() {
			valid = append(valid, it)
		} else {
			dropped++
			log.Printf("distill: candidato descartado (type=%q title=%q)", it.Type, it.Title)
		}
	}
	return valid, dropped
}
