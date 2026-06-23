package store

// engine_runs marca quais sessões já foram processadas por cada motor, para o
// scheduler rodar cada (motor, sessão) no máximo uma vez (sem re-emitir
// sugestões a cada tick). Chave (engine_id, session_id).

// UnrunEndedSessions devolve as sessões encerradas que o motor engineID ainda
// NÃO processou. projectID=="" abrange todos os projetos. O scheduler resolve a
// config por projeto e decide se roda.
func (s *Store) UnrunEndedSessions(engineID, projectID string) ([]*Session, error) {
	rows, err := s.db.Query(`SELECT id, COALESCE(project_id,''), adapter, mode, title, status,
		started_at, ended_at
		FROM sessions
		WHERE status='ended' AND project_id IS NOT NULL
		  AND id NOT IN (SELECT session_id FROM engine_runs WHERE engine_id=?)
		  AND (?='' OR project_id=?)
		ORDER BY ended_at`, engineID, projectID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Session{}
	for rows.Next() {
		x := &Session{}
		if err := rows.Scan(&x.ID, &x.ProjectID, &x.Adapter, &x.Mode, &x.Title, &x.Status,
			&x.StartedAt, &x.EndedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// CountUnrunEndedSessions conta as sessões encerradas não-analisadas por engineID
// (mesmo predicado de UnrunEndedSessions). projectID=="" abrange todos.
func (s *Store) CountUnrunEndedSessions(engineID, projectID string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions
		WHERE status='ended' AND project_id IS NOT NULL
		  AND id NOT IN (SELECT session_id FROM engine_runs WHERE engine_id=?)
		  AND (?='' OR project_id=?)`, engineID, projectID, projectID).Scan(&n)
	return n, err
}

// ActiveSessions devolve as sessões em andamento (status='active'). Usado pelo
// modo "ao vivo": o scheduler roda o motor sobre elas durante a sessão.
func (s *Store) ActiveSessions() ([]*Session, error) {
	rows, err := s.db.Query(`SELECT id, COALESCE(project_id,''), adapter, mode, title, status,
		started_at, ended_at FROM sessions WHERE status='active' AND project_id IS NOT NULL ORDER BY started_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Session{}
	for rows.Next() {
		x := &Session{}
		if err := rows.Scan(&x.ID, &x.ProjectID, &x.Adapter, &x.Mode, &x.Title, &x.Status,
			&x.StartedAt, &x.EndedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// MarkEngineRun registra que engineID processou sessionID (idempotente via OR IGNORE).
func (s *Store) MarkEngineRun(engineID, sessionID string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO engine_runs (engine_id, session_id, ran_at)
		VALUES (?,?,?)`, engineID, sessionID, now())
	return err
}
