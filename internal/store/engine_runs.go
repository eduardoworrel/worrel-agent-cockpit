package store

// engine_runs marca quais sessões já foram processadas por cada motor, para o
// scheduler rodar cada (motor, sessão) no máximo uma vez (sem re-emitir
// sugestões a cada tick). Chave (engine_id, session_id).

// UnrunEndedSessions devolve as sessões encerradas que o motor engineID ainda
// NÃO processou. O scheduler resolve a config por projeto e decide se roda.
func (s *Store) UnrunEndedSessions(engineID string) ([]*Session, error) {
	rows, err := s.db.Query(`SELECT id, COALESCE(project_id,''), adapter, mode, title, status,
		started_at, ended_at
		FROM sessions
		WHERE status='ended' AND id NOT IN (SELECT session_id FROM engine_runs WHERE engine_id=?)
		ORDER BY ended_at`, engineID)
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
