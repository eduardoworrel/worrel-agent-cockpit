package store

import (
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

type SkillCandidate struct {
	ID           string `json:"id"`
	ProjectID    string `json:"project_id"`
	Signature    string `json:"signature"`
	Title        string `json:"title"`
	Draft        string `json:"draft"`
	Evidence     string `json:"evidence"`
	Occurrences  int64  `json:"occurrences"`
	ExplicitMark int64  `json:"explicit_mark"`
	Status       string `json:"status"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
}

type CandidateOccurrence struct {
	SessionID string `json:"session_id"`
	SeqRange  string `json:"seq_range"`
	Signal    string `json:"signal"`
}

func (s *Store) getCandidate(projectID, signature string) (*SkillCandidate, error) {
	c := &SkillCandidate{}
	err := s.db.QueryRow(`SELECT id, project_id, signature, title, draft, evidence, occurrences, explicit_mark, status, created_at, updated_at
		FROM skill_candidates WHERE project_id=? AND signature=?`, projectID, signature).
		Scan(&c.ID, &c.ProjectID, &c.Signature, &c.Title, &c.Draft, &c.Evidence, &c.Occurrences, &c.ExplicitMark, &c.Status, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

// upsertCandidate é a base compartilhada: cria o candidato se a assinatura é nova; senão,
// se a session_id de occ ainda não está na evidência, incrementa occurrences e
// anexa a evidência. Idempotente por (project_id, signature) + session_id.
func (s *Store) upsertCandidate(projectID, signature, title, draft string, occ CandidateOccurrence, explicit bool) (*SkillCandidate, error) {
	c, err := s.getCandidate(projectID, signature)
	if err == sql.ErrNoRows {
		var ev []CandidateOccurrence
		if occ.SessionID != "" {
			ev = append(ev, occ)
		}
		evJSON, _ := json.Marshal(ev)
		nc := &SkillCandidate{
			ID: uuid.NewString(), ProjectID: projectID, Signature: signature, Title: title,
			Draft: draft, Evidence: string(evJSON), Occurrences: int64(len(ev)), Status: "accumulating",
			CreatedAt: now(), UpdatedAt: now(),
		}
		if explicit {
			nc.ExplicitMark = 1
		}
		_, err := s.db.Exec(`INSERT INTO skill_candidates
			(id, project_id, signature, title, draft, evidence, occurrences, explicit_mark, status, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			nc.ID, nc.ProjectID, nc.Signature, nc.Title, nc.Draft, nc.Evidence, nc.Occurrences, nc.ExplicitMark, nc.Status, nc.CreatedAt, nc.UpdatedAt)
		return nc, err
	}
	if err != nil {
		return nil, err
	}
	// existe: dedupe por session_id
	var ev []CandidateOccurrence
	_ = json.Unmarshal([]byte(c.Evidence), &ev)
	seen := false
	for _, e := range ev {
		if e.SessionID != "" && e.SessionID == occ.SessionID {
			seen = true
			break
		}
	}
	if !seen && occ.SessionID != "" {
		ev = append(ev, occ)
		c.Occurrences = int64(len(ev))
	}
	if explicit {
		c.ExplicitMark = 1
	}
	if strings.TrimSpace(draft) != "" && draft != "{}" {
		c.Draft = draft
	}
	if title != "" {
		c.Title = title
	}
	evJSON, _ := json.Marshal(ev)
	c.Evidence = string(evJSON)
	c.UpdatedAt = now()
	_, err = s.db.Exec(`UPDATE skill_candidates SET title=?, draft=?, evidence=?, occurrences=?, explicit_mark=?, updated_at=?
		WHERE id=?`, c.Title, c.Draft, c.Evidence, c.Occurrences, c.ExplicitMark, c.UpdatedAt, c.ID)
	return c, err
}

func (s *Store) UpsertSkillCandidate(projectID, signature, title, draft string, occ CandidateOccurrence) (*SkillCandidate, error) {
	return s.upsertCandidate(projectID, signature, title, draft, occ, false)
}

func (s *Store) MarkCandidateExplicit(projectID, signature, title, draft string, occ CandidateOccurrence) (*SkillCandidate, error) {
	return s.upsertCandidate(projectID, signature, title, draft, occ, true)
}

func (s *Store) ListSkillCandidates(projectID, status string) ([]*SkillCandidate, error) {
	q := `SELECT id, project_id, signature, title, draft, evidence, occurrences, explicit_mark, status, created_at, updated_at
		FROM skill_candidates WHERE project_id=?`
	args := []any{projectID}
	if status != "" {
		q += ` AND status=?`
		args = append(args, status)
	}
	q += ` ORDER BY updated_at DESC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*SkillCandidate{}
	for rows.Next() {
		c := &SkillCandidate{}
		if err := rows.Scan(&c.ID, &c.ProjectID, &c.Signature, &c.Title, &c.Draft, &c.Evidence, &c.Occurrences, &c.ExplicitMark, &c.Status, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) MatureSkillCandidate(id string) error {
	_, err := s.db.Exec(`UPDATE skill_candidates SET status='matured', updated_at=? WHERE id=?`, now(), id)
	return err
}

func (s *Store) DismissSkillCandidate(id string) error {
	_, err := s.db.Exec(`UPDATE skill_candidates SET status='dismissed', updated_at=? WHERE id=?`, now(), id)
	return err
}
