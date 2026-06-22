package store

// engine_log registra cada execução de um motor para explicabilidade: quando
// rodou, em qual sessão/projeto, sob qual gatilho, quantas sugestões gerou e
// quais (títulos). Para chamadas de IA grava também o prompt (input) e a
// resposta crua do modelo (output). Alimenta a aba "Atividade" da config.

type EngineLogEntry struct {
	ID          int64  `json:"id"`
	EngineID    string `json:"engine_id"`
	ProjectID   string `json:"project_id"`
	SessionID   string `json:"session_id"`
	Trigger     string `json:"trigger"`
	Suggestions int    `json:"suggestions"`
	Detail      string `json:"detail"`
	Input       string `json:"input"`  // prompt enviado à IA ('' = sem IA)
	Output      string `json:"output"` // resposta crua do modelo ('' = sem IA)
	CreatedAt   int64  `json:"created_at"`
}

func (s *Store) LogEngineRun(e *EngineLogEntry) error {
	res, err := s.db.Exec(`INSERT INTO engine_log
		(engine_id, project_id, session_id, trigger, suggestions, detail, input, output, created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		e.EngineID, e.ProjectID, e.SessionID, e.Trigger, e.Suggestions, e.Detail,
		e.Input, e.Output, now())
	if err != nil {
		return err
	}
	e.ID, _ = res.LastInsertId()
	return nil
}

// ListEngineLog devolve as execuções mais recentes (até limit), das mais novas
// para as mais antigas.
func (s *Store) ListEngineLog(limit int) ([]*EngineLogEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT id, engine_id, COALESCE(project_id,''), COALESCE(session_id,''),
		trigger, suggestions, detail, input, output, created_at
		FROM engine_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*EngineLogEntry{}
	for rows.Next() {
		e := &EngineLogEntry{}
		if err := rows.Scan(&e.ID, &e.EngineID, &e.ProjectID, &e.SessionID,
			&e.Trigger, &e.Suggestions, &e.Detail, &e.Input, &e.Output, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
