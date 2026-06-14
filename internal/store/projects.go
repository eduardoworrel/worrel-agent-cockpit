package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

type Project struct {
	ID           string   `json:"id"`
	Slug         string   `json:"slug"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Dirs         []string `json:"dirs"`
	CreatedAt    int64    `json:"created_at"`
	UpdatedAt    int64    `json:"updated_at"`
	WorkspaceDir string   `json:"workspace_dir"`
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func Slugify(s string) string {
	s = strings.ToLower(s)
	s = nonSlug.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func (s *Store) CreateProject(name, description string) (*Project, error) {
	base := Slugify(name)
	if base == "" {
		base = "projeto"
	}
	slug := base
	for i := 2; ; i++ {
		var n int
		if err := s.db.QueryRow(`SELECT count(*) FROM projects WHERE slug=?`, slug).Scan(&n); err != nil {
			return nil, err
		}
		if n == 0 {
			break
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
	ws := filepath.Join(s.dataDir, "workspaces", slug)
	p := &Project{ID: uuid.NewString(), Slug: slug, Name: name, Description: description,
		Dirs: []string{}, CreatedAt: now(), UpdatedAt: now(), WorkspaceDir: ws}
	_, err := s.db.Exec(`INSERT INTO projects (id, slug, name, description, workspace_dir, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?)`, p.ID, p.Slug, p.Name, p.Description, p.WorkspaceDir, p.CreatedAt, p.UpdatedAt)
	return p, err
}

func (s *Store) scanProject(row *sql.Row) (*Project, error) {
	p := &Project{Dirs: []string{}}
	if err := row.Scan(&p.ID, &p.Slug, &p.Name, &p.Description, &p.CreatedAt, &p.UpdatedAt, &p.WorkspaceDir); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT dir FROM project_dirs WHERE project_id=? ORDER BY dir`, p.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		p.Dirs = append(p.Dirs, d)
	}
	return p, rows.Err()
}

func (s *Store) GetProject(id string) (*Project, error) {
	return s.scanProject(s.db.QueryRow(
		`SELECT id, slug, name, description, created_at, updated_at, workspace_dir FROM projects WHERE id=?`, id))
}

func (s *Store) ProjectByDir(dir string) (*Project, error) {
	var id string
	if err := s.db.QueryRow(`SELECT project_id FROM project_dirs WHERE dir=?`, dir).Scan(&id); err != nil {
		return nil, err
	}
	return s.GetProject(id)
}

func (s *Store) ListProjects() ([]*Project, error) {
	rows, err := s.db.Query(`SELECT id FROM projects ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []*Project{}
	for _, id := range ids {
		p, err := s.GetProject(id)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *Store) UpdateProject(id, name, description string) error {
	res, err := s.db.Exec(`UPDATE projects SET name=?, description=?, updated_at=? WHERE id=?`,
		name, description, now(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) TouchProject(id string) error {
	_, err := s.db.Exec(`UPDATE projects SET updated_at=? WHERE id=?`, now(), id)
	return err
}

func (s *Store) AddProjectDir(id, dir string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO project_dirs (project_id, dir) VALUES (?,?)`, id, dir)
	return err
}

func (s *Store) RemoveProjectDir(id, dir string) error {
	_, err := s.db.Exec(`DELETE FROM project_dirs WHERE project_id=? AND dir=?`, id, dir)
	return err
}

func (s *Store) DeleteProject(id string) error {
	_, err := s.db.Exec(`DELETE FROM projects WHERE id=?`, id)
	return err
}
