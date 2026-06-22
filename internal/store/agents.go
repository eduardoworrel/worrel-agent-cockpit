package store

import (
	"regexp"
	"strings"

	"github.com/google/uuid"
)

type Agent struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Persona   string `json:"persona"`
	Evidence  string `json:"evidence"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
	ActiveGeneration int64 `json:"active_generation"`
}

var agentSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

func agentSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = agentSlugRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func (s *Store) CreateAgent(projectID, name, persona, evidence string) (*Agent, error) {
	a := &Agent{
		ID: uuid.NewString(), ProjectID: projectID, Slug: agentSlug(name),
		Name: name, Persona: persona, Evidence: evidence, CreatedAt: now(), UpdatedAt: now(),
	}
	_, err := s.db.Exec(`INSERT INTO agents (id, project_id, slug, name, persona, evidence, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?)`, a.ID, a.ProjectID, a.Slug, a.Name, a.Persona, a.Evidence, a.CreatedAt, a.UpdatedAt)
	if err == nil {
		a.ActiveGeneration = 1
		_ = s.SeedAgentGeneration(a.ID, a.Persona)
	}
	return a, err
}

func (s *Store) GetAgent(id string) (*Agent, error) {
	a := &Agent{}
	err := s.db.QueryRow(`SELECT id, project_id, slug, name, persona, evidence, created_at, updated_at, COALESCE(active_generation,1)
		FROM agents WHERE id=?`, id).Scan(&a.ID, &a.ProjectID, &a.Slug, &a.Name, &a.Persona, &a.Evidence, &a.CreatedAt, &a.UpdatedAt, &a.ActiveGeneration)
	return a, err
}

func (s *Store) ListAgents(projectID string) ([]*Agent, error) {
	rows, err := s.db.Query(`SELECT id, project_id, slug, name, persona, evidence, created_at, updated_at, COALESCE(active_generation,1)
		FROM agents WHERE project_id=? ORDER BY created_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Agent{}
	for rows.Next() {
		a := &Agent{}
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.Slug, &a.Name, &a.Persona, &a.Evidence, &a.CreatedAt, &a.UpdatedAt, &a.ActiveGeneration); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) DeleteAgent(id string) error {
	_, err := s.db.Exec(`DELETE FROM agents WHERE id=?`, id)
	return err
}
