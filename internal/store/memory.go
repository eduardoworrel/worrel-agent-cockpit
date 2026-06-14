package store

import "database/sql"

type MemoryVersion struct {
	ID        int64  `json:"id"`
	ProjectID string `json:"project_id"`
	Content   string `json:"content"`
	Note      string `json:"note"`
	CreatedAt int64  `json:"created_at"`
}

// GetMemory devolve a versão mais recente (ou vazia se não houver).
func (s *Store) GetMemory(projectID string) (*MemoryVersion, error) {
	row := s.db.QueryRow(`SELECT id, project_id, content, note, created_at FROM memory_versions
		WHERE project_id=? ORDER BY id DESC LIMIT 1`, projectID)
	v := &MemoryVersion{}
	err := row.Scan(&v.ID, &v.ProjectID, &v.Content, &v.Note, &v.CreatedAt)
	if err == sql.ErrNoRows {
		return &MemoryVersion{ProjectID: projectID}, nil
	}
	return v, err
}

func (s *Store) SaveMemory(projectID, content, note string) (*MemoryVersion, error) {
	ts := now()
	res, err := s.db.Exec(`INSERT INTO memory_versions (project_id, content, note, created_at)
		VALUES (?,?,?,?)`, projectID, content, note, ts)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	_ = s.TouchProject(projectID)
	return &MemoryVersion{ID: id, ProjectID: projectID, Content: content, Note: note, CreatedAt: ts}, nil
}

func (s *Store) ListMemoryVersions(projectID string) ([]*MemoryVersion, error) {
	rows, err := s.db.Query(`SELECT id, project_id, content, note, created_at FROM memory_versions
		WHERE project_id=? ORDER BY id DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*MemoryVersion{}
	for rows.Next() {
		v := &MemoryVersion{}
		if err := rows.Scan(&v.ID, &v.ProjectID, &v.Content, &v.Note, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) RevertMemory(projectID string, versionID int64) (*MemoryVersion, error) {
	var content string
	if err := s.db.QueryRow(`SELECT content FROM memory_versions WHERE id=? AND project_id=?`,
		versionID, projectID).Scan(&content); err != nil {
		return nil, err
	}
	return s.SaveMemory(projectID, content, "revert")
}
