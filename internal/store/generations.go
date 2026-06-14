package store

import "database/sql"

type SkillGeneration struct {
	ID             int64    `json:"id"`
	SkillID        string   `json:"skill_id"`
	Generation     int64    `json:"generation"`
	EvolutionType  string   `json:"evolution_type"`
	ParentSkillIDs []string `json:"parent_skill_ids"`
	Diff           string   `json:"diff"`
	Snapshot       string   `json:"snapshot"`
	ChangeSummary  string   `json:"change_summary"`
	Evidence       string   `json:"evidence"`
	Authorship     string   `json:"authorship"`
	CreatedAt      int64    `json:"created_at"`
}

type GenerationInput struct {
	EvolutionType  string
	ParentSkillIDs []string
	Diff           string
	Snapshot       string
	ChangeSummary  string
	Evidence       string
	Authorship     string
}

func (s *Store) SeedGeneration(skillID, snapshot string) error {
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM skill_generations WHERE skill_id=? AND generation=1`, skillID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err := s.db.Exec(`INSERT INTO skill_generations (skill_id, generation, evolution_type, snapshot, authorship, created_at)
		VALUES (?, 1, 'learned', ?, 'human', ?)`, skillID, snapshot, now())
	return err
}

func (s *Store) AddGeneration(skillID string, inp GenerationInput) (*SkillGeneration, error) {
	var maxGen int64
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(generation),0) FROM skill_generations WHERE skill_id=?`, skillID).Scan(&maxGen); err != nil {
		return nil, err
	}
	gen := maxGen + 1
	authorship := inp.Authorship
	if authorship == "" {
		authorship = "human"
	}
	parentJSON := "[]"
	if len(inp.ParentSkillIDs) > 0 {
		b, _ := marshalJSON(inp.ParentSkillIDs)
		parentJSON = string(b)
	}
	_, err := s.db.Exec(`INSERT INTO skill_generations (skill_id, generation, evolution_type, parent_skill_ids, diff, snapshot, change_summary, evidence, authorship, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		skillID, gen, inp.EvolutionType, parentJSON, inp.Diff, inp.Snapshot, inp.ChangeSummary, inp.Evidence, authorship, now())
	if err != nil {
		return nil, err
	}
	if err := s.activate(skillID, gen, inp.Snapshot); err != nil {
		return nil, err
	}
	return s.getGeneration(skillID, gen)
}

func (s *Store) ActivateGeneration(skillID string, generation int64) error {
	var snapshot string
	err := s.db.QueryRow(`SELECT snapshot FROM skill_generations WHERE skill_id=? AND generation=?`, skillID, generation).Scan(&snapshot)
	if err != nil {
		return err
	}
	return s.activate(skillID, generation, snapshot)
}

func (s *Store) activate(skillID string, generation int64, snapshot string) error {
	_, err := s.db.Exec(`UPDATE skills SET active_generation=?, content=?, updated_at=? WHERE id=?`,
		generation, snapshot, now(), skillID)
	return err
}

func (s *Store) ListGenerations(skillID string) ([]*SkillGeneration, error) {
	rows, err := s.db.Query(`SELECT id, skill_id, generation, evolution_type, parent_skill_ids, diff, snapshot, change_summary, evidence, authorship, created_at
		FROM skill_generations WHERE skill_id=? ORDER BY generation ASC`, skillID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*SkillGeneration
	for rows.Next() {
		g, err := scanGeneration(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) getGeneration(skillID string, generation int64) (*SkillGeneration, error) {
	return scanGeneration(s.db.QueryRow(`SELECT id, skill_id, generation, evolution_type, parent_skill_ids, diff, snapshot, change_summary, evidence, authorship, created_at
		FROM skill_generations WHERE skill_id=? AND generation=?`, skillID, generation))
}

func scanGeneration(r rowScanner) (*SkillGeneration, error) {
	g := &SkillGeneration{}
	var parentJSON string
	err := r.Scan(&g.ID, &g.SkillID, &g.Generation, &g.EvolutionType, &parentJSON,
		&g.Diff, &g.Snapshot, &g.ChangeSummary, &g.Evidence, &g.Authorship, &g.CreatedAt)
	if err != nil {
		return nil, err
	}
	g.ParentSkillIDs = unmarshalStringSlice(parentJSON)
	return g, nil
}

func (s *Store) GenerationsWithParent(parentSkillID string) ([]*SkillGeneration, error) {
	rows, err := s.db.Query(`SELECT id, skill_id, generation, evolution_type, parent_skill_ids, diff, snapshot, change_summary, evidence, authorship, created_at
		FROM skill_generations WHERE parent_skill_ids LIKE ?`, "%"+parentSkillID+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*SkillGeneration
	for rows.Next() {
		g, err := scanGeneration(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) RewriteSeedAsVariant(skillID string, parentIDs []string, evidence, changeSummary string) error {
	parentJSON := "[]"
	if len(parentIDs) > 0 {
		b, _ := marshalJSON(parentIDs)
		parentJSON = string(b)
	}
	_, err := s.db.Exec(`UPDATE skill_generations SET evolution_type='variant', parent_skill_ids=?, evidence=?, change_summary=? WHERE skill_id=? AND generation=1`,
		parentJSON, evidence, changeSummary, skillID)
	return err
}

func (s *Store) CountAutoAppliedToday(skillID string, since int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT count(*) FROM skill_generations
		WHERE skill_id=? AND authorship='engine_auto' AND created_at>=?`, skillID, since).Scan(&n)
	return n, err
}

// helpers for JSON marshal/unmarshal without encoding/json import cycle
func marshalJSON(ss []string) ([]byte, error) {
	if len(ss) == 0 {
		return []byte("[]"), nil
	}
	b := []byte{'['}
	for i, s := range ss {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '"')
		for _, c := range s {
			if c == '"' || c == '\\' {
				b = append(b, '\\')
			}
			b = append(b, byte(c))
		}
		b = append(b, '"')
	}
	b = append(b, ']')
	return b, nil
}

func unmarshalStringSlice(s string) []string {
	if s == "" || s == "[]" || s == "null" {
		return []string{}
	}
	// simple JSON array of strings parser
	var out []string
	i := 0
	for i < len(s) {
		if s[i] == '"' {
			i++
			start := i
			for i < len(s) && s[i] != '"' {
				if s[i] == '\\' {
					i++
				}
				i++
			}
			out = append(out, s[start:i])
			i++
		} else {
			i++
		}
	}
	return out
}

// ensure sql is imported
var _ = sql.ErrNoRows
