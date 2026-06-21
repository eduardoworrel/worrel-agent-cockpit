package store

import (
	"sort"
	"strings"

	"github.com/google/uuid"
)

type MemoryEntry struct {
	ID           string `json:"id"`
	ProjectID    string `json:"project_id"`
	Content      string `json:"content"`
	Category     string `json:"category"`
	Evidence     string `json:"evidence"`
	Status       string `json:"status"`        // active | superseded
	SupersededBy string `json:"superseded_by"` // id da entrada que substituiu, ou ""
	CreatedAt    int64  `json:"created_at"`
}

// categoryLabels mapeia a categoria-chave para o cabeçalho do render.
var categoryLabels = map[string]string{
	"convencao":   "Convenções",
	"arquitetura": "Arquitetura",
	"gotcha":      "Gotchas",
	"never_do":    "Nunca faça",
	"decisao":     "Decisões",
}

// categoryOrder fixa a ordem das seções no render.
var categoryOrder = []string{"convencao", "arquitetura", "gotcha", "never_do", "decisao"}

func (s *Store) CreateMemoryEntry(e *MemoryEntry) (*MemoryEntry, error) {
	e.ID = uuid.NewString()
	if e.Category == "" {
		e.Category = "gotcha"
	}
	e.Status = "active"
	e.CreatedAt = now()
	_, err := s.db.Exec(`INSERT INTO memory_entries
		(id, project_id, content, category, evidence, status, superseded_by, created_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		e.ID, e.ProjectID, e.Content, e.Category, e.Evidence, e.Status, "", e.CreatedAt)
	return e, err
}

func (s *Store) ListMemoryEntries(projectID string, includeSuperseded bool) ([]*MemoryEntry, error) {
	q := `SELECT id, project_id, content, category, evidence, status, superseded_by, created_at
		FROM memory_entries WHERE project_id=?`
	if !includeSuperseded {
		q += ` AND status='active'`
	}
	q += ` ORDER BY created_at`
	rows, err := s.db.Query(q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*MemoryEntry{}
	for rows.Next() {
		e := &MemoryEntry{}
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.Content, &e.Category, &e.Evidence,
			&e.Status, &e.SupersededBy, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// SupersedeMemoryEntry marca oldID como superseded apontando para newID.
func (s *Store) SupersedeMemoryEntry(oldID, newID string) error {
	_, err := s.db.Exec(`UPDATE memory_entries SET status='superseded', superseded_by=? WHERE id=?`,
		newID, oldID)
	return err
}

// RenderMemory concatena as entradas ativas agrupadas por categoria num markdown
// legível, consumido pela injeção-no-início. Vazio se não houver entradas.
func (s *Store) RenderMemory(projectID string) (string, error) {
	entries, err := s.ListMemoryEntries(projectID, false)
	if err != nil {
		return "", err
	}
	byCat := map[string][]string{}
	for _, e := range entries {
		byCat[e.Category] = append(byCat[e.Category], e.Content)
	}
	var b strings.Builder
	for _, cat := range categoryOrder {
		items := byCat[cat]
		if len(items) == 0 {
			continue
		}
		b.WriteString("## " + categoryLabels[cat] + "\n")
		for _, it := range items {
			b.WriteString("- " + it + "\n")
		}
		b.WriteString("\n")
	}
	// categorias desconhecidas (defensivo): em ordem estável
	var unknown []string
	for cat := range byCat {
		if _, ok := categoryLabels[cat]; !ok {
			unknown = append(unknown, cat)
		}
	}
	sort.Strings(unknown)
	for _, cat := range unknown {
		b.WriteString("## " + cat + "\n")
		for _, it := range byCat[cat] {
			b.WriteString("- " + it + "\n")
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String()), nil
}
