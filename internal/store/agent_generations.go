package store

import "database/sql"

type AgentGeneration struct {
	ID            int64  `json:"id"`
	AgentID       string `json:"agent_id"`
	Generation    int64  `json:"generation"`
	Persona       string `json:"persona"`
	ChangeSummary string `json:"change_summary"`
	Evidence      string `json:"evidence"`
	CreatedAt     int64  `json:"created_at"`
}

// SeedAgentGeneration cria a geração 1 se ainda não houver nenhuma (idempotente).
func (s *Store) SeedAgentGeneration(agentID, persona string) error {
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM agent_generations WHERE agent_id=?`, agentID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err := s.db.Exec(`INSERT INTO agent_generations (agent_id, generation, persona, created_at)
		VALUES (?, 1, ?, ?)`, agentID, persona, now())
	return err
}

// AddAgentGeneration cria a próxima geração e a ativa (atualiza agents.persona +
// active_generation). Espelha AddGeneration de skills.
func (s *Store) AddAgentGeneration(agentID, persona, changeSummary, evidence string) (*AgentGeneration, error) {
	var maxGen sql.NullInt64
	if err := s.db.QueryRow(`SELECT MAX(generation) FROM agent_generations WHERE agent_id=?`, agentID).Scan(&maxGen); err != nil {
		return nil, err
	}
	gen := maxGen.Int64 + 1
	ts := now()
	res, err := s.db.Exec(`INSERT INTO agent_generations (agent_id, generation, persona, change_summary, evidence, created_at)
		VALUES (?,?,?,?,?,?)`, agentID, gen, persona, changeSummary, evidence, ts)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	if _, err := s.db.Exec(`UPDATE agents SET persona=?, active_generation=?, updated_at=? WHERE id=?`,
		persona, gen, ts, agentID); err != nil {
		return nil, err
	}
	return &AgentGeneration{ID: id, AgentID: agentID, Generation: gen, Persona: persona,
		ChangeSummary: changeSummary, Evidence: evidence, CreatedAt: ts}, nil
}

func (s *Store) ListAgentGenerations(agentID string) ([]*AgentGeneration, error) {
	rows, err := s.db.Query(`SELECT id, agent_id, generation, persona, change_summary, evidence, created_at
		FROM agent_generations WHERE agent_id=? ORDER BY generation`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*AgentGeneration{}
	for rows.Next() {
		g := &AgentGeneration{}
		if err := rows.Scan(&g.ID, &g.AgentID, &g.Generation, &g.Persona, &g.ChangeSummary, &g.Evidence, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
