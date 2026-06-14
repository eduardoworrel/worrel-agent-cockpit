package store

import (
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

type Skill struct {
	ID               string `json:"id"`
	ProjectID        string `json:"project_id"`
	Slug             string `json:"slug"`
	Name             string `json:"name"`
	Content          string `json:"content"`
	CreatedAt        int64  `json:"created_at"`
	UpdatedAt        int64  `json:"updated_at"`
	ActiveGeneration int64  `json:"active_generation"`
	EvolutionPolicy  string `json:"evolution_policy"`
	Origin           string `json:"origin"`
}

func (s *Store) CreateSkill(projectID, name, content string) (*Skill, error) {
	base := Slugify(name)
	if base == "" {
		base = "skill"
	}
	slug := base
	for i := 2; ; i++ {
		var n int
		if err := s.db.QueryRow(`SELECT count(*) FROM skills WHERE project_id=? AND slug=?`,
			projectID, slug).Scan(&n); err != nil {
			return nil, err
		}
		if n == 0 {
			break
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
	sk := &Skill{ID: uuid.NewString(), ProjectID: projectID, Slug: slug, Name: name,
		Content: content, CreatedAt: now(), UpdatedAt: now()}
	_, err := s.db.Exec(`INSERT INTO skills (id, project_id, slug, name, content, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?)`, sk.ID, sk.ProjectID, sk.Slug, sk.Name, sk.Content, sk.CreatedAt, sk.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if err := s.SeedGeneration(sk.ID, sk.Content); err != nil {
		return nil, err
	}
	sk.ActiveGeneration = 1
	sk.EvolutionPolicy = "manual"
	sk.Origin = "learned"
	return sk, nil
}

func (s *Store) GetSkill(id string) (*Skill, error) {
	sk := &Skill{}
	err := s.db.QueryRow(`SELECT id, project_id, slug, name, content, created_at, updated_at,
		COALESCE(active_generation,1), COALESCE(evolution_policy,'manual'), COALESCE(origin,'learned')
		FROM skills WHERE id=?`, id).
		Scan(&sk.ID, &sk.ProjectID, &sk.Slug, &sk.Name, &sk.Content, &sk.CreatedAt, &sk.UpdatedAt,
			&sk.ActiveGeneration, &sk.EvolutionPolicy, &sk.Origin)
	if err != nil {
		return nil, err
	}
	return sk, nil
}

// ListSkills com projectID vazio lista todas.
func (s *Store) ListSkills(projectID string) ([]*Skill, error) {
	q := `SELECT id, project_id, slug, name, content, created_at, updated_at,
		COALESCE(active_generation,1), COALESCE(evolution_policy,'manual'), COALESCE(origin,'learned') FROM skills`
	args := []any{}
	if projectID != "" {
		q += ` WHERE project_id=?`
		args = append(args, projectID)
	}
	q += ` ORDER BY updated_at DESC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Skill{}
	for rows.Next() {
		sk := &Skill{}
		if err := rows.Scan(&sk.ID, &sk.ProjectID, &sk.Slug, &sk.Name, &sk.Content,
			&sk.CreatedAt, &sk.UpdatedAt, &sk.ActiveGeneration, &sk.EvolutionPolicy, &sk.Origin); err != nil {
			return nil, err
		}
		out = append(out, sk)
	}
	return out, rows.Err()
}

func (s *Store) UpdateSkill(id, name, content string) error {
	res, err := s.db.Exec(`UPDATE skills SET name=?, content=?, updated_at=? WHERE id=?`,
		name, content, now(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// CreateSkillWithOrigin cria uma skill com origem e política personalizadas.
func (s *Store) CreateSkillWithOrigin(projectID, name, content, origin, policy string) (*Skill, error) {
	sk, err := s.CreateSkill(projectID, name, content)
	if err != nil {
		return nil, err
	}
	if origin != "" {
		if _, err := s.db.Exec(`UPDATE skills SET origin=? WHERE id=?`, origin, sk.ID); err != nil {
			return nil, err
		}
		sk.Origin = origin
	}
	if policy != "" {
		if _, err := s.db.Exec(`UPDATE skills SET evolution_policy=? WHERE id=?`, policy, sk.ID); err != nil {
			return nil, err
		}
		sk.EvolutionPolicy = policy
	}
	return sk, nil
}

func (s *Store) SetSkillPolicy(id, policy string) error {
	_, err := s.db.Exec(`UPDATE skills SET evolution_policy=? WHERE id=?`, policy, id)
	return err
}

func (s *Store) SetProjectSkillsPolicy(projectID, policy string) error {
	_, err := s.db.Exec(`UPDATE skills SET evolution_policy=? WHERE project_id=?`, policy, projectID)
	return err
}

func (s *Store) RenameSkill(id, name string) error {
	_, err := s.db.Exec(`UPDATE skills SET name=?, updated_at=? WHERE id=?`, name, now(), id)
	return err
}

func (s *Store) DeleteSkill(id string) error {
	res, err := s.db.Exec(`DELETE FROM skills WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
