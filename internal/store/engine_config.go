package store

// SetEngineConfig grava (upsert) um par chave/valor de config de motor.
// projectID "" = escopo global; qualquer outro valor = override de projeto.
func (s *Store) SetEngineConfig(engineID, key, value, projectID string) error {
	_, err := s.db.Exec(`INSERT INTO engine_config (engine_id, scope_key, key, value, updated_at)
		VALUES (?,?,?,?,?)
		ON CONFLICT(engine_id, scope_key, key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		engineID, projectID, key, value, now())
	return err
}

// GetEngineConfig devolve as linhas cruas de UM escopo (sem mesclar camadas).
func (s *Store) GetEngineConfig(engineID, projectID string) (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key, value FROM engine_config WHERE engine_id=? AND scope_key=?`,
		engineID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// ResolveEngineConfig mescla as camadas: defaults ⊕ global ('') ⊕ projeto.
func (s *Store) ResolveEngineConfig(engineID, projectID string, defaults map[string]string) (map[string]string, error) {
	out := map[string]string{}
	for k, v := range defaults {
		out[k] = v
	}
	global, err := s.GetEngineConfig(engineID, "")
	if err != nil {
		return nil, err
	}
	for k, v := range global {
		out[k] = v
	}
	if projectID != "" {
		proj, err := s.GetEngineConfig(engineID, projectID)
		if err != nil {
			return nil, err
		}
		for k, v := range proj {
			out[k] = v
		}
	}
	return out, nil
}
