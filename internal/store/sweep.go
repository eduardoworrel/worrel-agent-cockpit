package store

// NOTE: sweep.go ONLY adds new query functions. It must NOT redeclare or edit
// sessionCols/scanSession — those symbols are owned by sessions.go (fase 6 is
// the single owner of sessionCols/scanSession; see amendment B1 in fase 6 plan).

// PendingSweepSessions: sessões a analisar (não analisadas, encerradas/observadas).
func (s *Store) PendingSweepSessions() ([]*Session, error) {
	rows, err := s.db.Query(sessionCols +
		` WHERE analyzed_at IS NULL AND status IN ('ended','observed') ORDER BY started_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Session{}
	for rows.Next() {
		x, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) MarkSessionAnalyzed(id string) error {
	_, err := s.db.Exec(`UPDATE sessions SET analyzed_at=? WHERE id=?`, now(), id)
	return err
}

// RecentlyUpdatedSkills: skills atualizadas nas últimas `hours` horas (dedupe §7.3).
func (s *Store) RecentlyUpdatedSkills(hours int64) ([]*Skill, error) {
	cutoff := now() - hours*3600*1000
	rows, err := s.db.Query(`SELECT id, project_id, slug, name, content, created_at, updated_at
		FROM skills WHERE updated_at >= ? ORDER BY updated_at DESC`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Skill{}
	for rows.Next() {
		sk := &Skill{}
		if err := rows.Scan(&sk.ID, &sk.ProjectID, &sk.Slug, &sk.Name, &sk.Content,
			&sk.CreatedAt, &sk.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, sk)
	}
	return out, rows.Err()
}
