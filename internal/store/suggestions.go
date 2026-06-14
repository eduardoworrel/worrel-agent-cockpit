package store

import (
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

// Tipos: create_project | add_memory | add_correction | create_skill | update_skill
// Status: pending | accepted | rejected | deferred
//
// CreateSuggestion ALWAYS starts as "pending" — caller-supplied Status and CreatedAt are overwritten by design.
type Suggestion struct {
	ID         string  `json:"id"`
	ProjectID  string  `json:"project_id"`
	SessionID  *string `json:"session_id"`
	SkillID    *string `json:"skill_id"`
	Type       string  `json:"type"`
	Status     string  `json:"status"`
	Title      string  `json:"title"`
	Payload    string  `json:"payload"`
	Evidence   string  `json:"evidence"`
	Origin     string  `json:"origin"` // incremental | retroativa
	CreatedAt  int64   `json:"created_at"`
	ResolvedAt *int64  `json:"resolved_at"`
}

func (s *Store) CreateSuggestion(sg *Suggestion) (*Suggestion, error) {
	sg.ID = uuid.NewString()
	sg.Status = "pending"
	sg.CreatedAt = now()
	if sg.Payload == "" {
		sg.Payload = "{}"
	}
	if sg.Origin == "" {
		sg.Origin = "incremental"
	}
	_, err := s.db.Exec(`INSERT INTO suggestions
		(id, project_id, session_id, skill_id, type, status, title, payload, evidence, origin, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		sg.ID, nullable(sg.ProjectID), sg.SessionID, sg.SkillID, sg.Type, sg.Status,
		sg.Title, sg.Payload, sg.Evidence, sg.Origin, sg.CreatedAt)
	return sg, err
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *Store) GetSuggestion(id string) (*Suggestion, error) {
	return scanSuggestion(s.db.QueryRow(`SELECT id, COALESCE(project_id,''), session_id, skill_id,
		type, status, title, payload, evidence, COALESCE(origin,'incremental'), created_at, resolved_at FROM suggestions WHERE id=?`, id))
}

type rowScanner interface{ Scan(dest ...any) error }

func scanSuggestion(r rowScanner) (*Suggestion, error) {
	sg := &Suggestion{}
	err := r.Scan(&sg.ID, &sg.ProjectID, &sg.SessionID, &sg.SkillID, &sg.Type, &sg.Status,
		&sg.Title, &sg.Payload, &sg.Evidence, &sg.Origin, &sg.CreatedAt, &sg.ResolvedAt)
	return sg, err
}

// ListSuggestions: projectID e status vazios = sem filtro.
func (s *Store) ListSuggestions(projectID, status string) ([]*Suggestion, error) {
	q := `SELECT id, COALESCE(project_id,''), session_id, skill_id, type, status, title, payload,
		evidence, COALESCE(origin,'incremental'), created_at, resolved_at FROM suggestions WHERE 1=1`
	args := []any{}
	if projectID != "" {
		q += ` AND project_id=?`
		args = append(args, projectID)
	}
	if status != "" {
		q += ` AND status=?`
		args = append(args, status)
	}
	q += ` ORDER BY created_at DESC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Suggestion{}
	for rows.Next() {
		sg, err := scanSuggestion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sg)
	}
	return out, rows.Err()
}

func (s *Store) ResolveSuggestion(id, status string) error {
	// Validate status only accepts valid values
	switch status {
	case "accepted", "rejected", "deferred", "auto_applied":
	default:
		return fmt.Errorf("status inválido: %s", status)
	}
	t := now()
	result, err := s.db.Exec(`UPDATE suggestions SET status=?, resolved_at=? WHERE id=?`, status, t, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ReclassifySuggestion altera o tipo de uma sugestão pending ou deferred.
func (s *Store) ReclassifySuggestion(id, newType string) error {
	result, err := s.db.Exec(`UPDATE suggestions SET type=? WHERE id=? AND status IN ('pending','deferred')`,
		newType, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateSuggestionPayload updates content only; status change is done separately via ResolveSuggestion.
// UpdateSuggestionContent atualiza título, payload e evidência (usado pela
// consolidação retroativa para registrar occurrences e concatenar evidências).
func (s *Store) UpdateSuggestionContent(id, title, payload, evidence string) error {
	result, err := s.db.Exec(`UPDATE suggestions SET title=?, payload=?, evidence=? WHERE id=?`,
		title, payload, evidence, id)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) UpdateSuggestionPayload(id, title, payload string) error {
	result, err := s.db.Exec(`UPDATE suggestions SET title=?, payload=? WHERE id=?`, title, payload, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}
