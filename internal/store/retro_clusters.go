package store

import "github.com/google/uuid"

type RetroCluster struct {
	ID                string  `json:"id"`
	RunID             string  `json:"run_id"`
	Name              string  `json:"name"`
	Description       string  `json:"description"`
	ExistingProjectID *string `json:"existing_project_id"`
	Dirs              string  `json:"dirs"`        // JSON []
	SessionIDs        string  `json:"session_ids"` // JSON []
	Decision          string  `json:"decision"`
	ApprovedProjectID *string `json:"approved_project_id"`
	CreatedAt         int64   `json:"created_at"`
}

func (s *Store) CreateRetroCluster(c *RetroCluster) (*RetroCluster, error) {
	c.ID = uuid.NewString()
	if c.Decision == "" {
		c.Decision = "pending"
	}
	if c.Dirs == "" {
		c.Dirs = "[]"
	}
	if c.SessionIDs == "" {
		c.SessionIDs = "[]"
	}
	c.CreatedAt = now()
	_, err := s.db.Exec(`INSERT INTO retro_clusters
		(id,run_id,name,description,existing_project_id,dirs,session_ids,decision,created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		c.ID, c.RunID, c.Name, c.Description, c.ExistingProjectID, c.Dirs, c.SessionIDs, c.Decision, c.CreatedAt)
	return c, err
}

const clusterCols = `SELECT id,run_id,name,description,existing_project_id,dirs,session_ids,decision,approved_project_id,created_at FROM retro_clusters`

func scanCluster(r rowScanner) (*RetroCluster, error) {
	c := &RetroCluster{}
	err := r.Scan(&c.ID, &c.RunID, &c.Name, &c.Description, &c.ExistingProjectID,
		&c.Dirs, &c.SessionIDs, &c.Decision, &c.ApprovedProjectID, &c.CreatedAt)
	return c, err
}

func (s *Store) ListRetroClusters(runID string) ([]*RetroCluster, error) {
	rows, err := s.db.Query(clusterCols+` WHERE run_id=? ORDER BY created_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*RetroCluster{}
	for rows.Next() {
		c, err := scanCluster(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetRetroCluster(id string) (*RetroCluster, error) {
	return scanCluster(s.db.QueryRow(clusterCols+` WHERE id=?`, id))
}

func (s *Store) SetClusterDecision(id, decision, approvedProjectID string) error {
	_, err := s.db.Exec(`UPDATE retro_clusters SET decision=?, approved_project_id=? WHERE id=?`,
		decision, nullable(approvedProjectID), id)
	return err
}

// --- supressão de segredos (critério 9; valor cru NUNCA armazenado) ---

func (s *Store) IsSecretSuppressed(hash string) bool {
	var n int
	_ = s.db.QueryRow(`SELECT count(*) FROM secret_suppressions WHERE hash=?`, hash).Scan(&n)
	return n > 0
}

func (s *Store) SuppressSecret(hash string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO secret_suppressions (hash, created_at) VALUES (?,?)`, hash, now())
	return err
}
