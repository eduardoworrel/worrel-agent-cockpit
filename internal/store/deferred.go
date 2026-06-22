package store

// Fila de adiadas: o usuário clica "Adiar" no modal de interação e a sessão
// recebe deferred_at = now. A bolinha no sidebar reabre o modal; responder
// limpa o marcador. Persistido na própria sessão para sobreviver a refresh e
// reinício do processo (igual às sessões).

// DeferredSession é o item enxuto consumido pelo sidebar: id, rótulo e quando
// foi adiada (para ordenar/agrupar as bolinhas).
type DeferredSession struct {
	SessionID  string `json:"session_id"`
	Label      string `json:"label"`
	ProjectID  string `json:"project_id"`
	DeferredAt int64  `json:"deferred_at"`
}

// SetSessionDeferred marca a sessão como adiada (deferred_at = now).
func (s *Store) SetSessionDeferred(sessionID string) error {
	_, err := s.db.Exec(`UPDATE sessions SET deferred_at=? WHERE id=?`, now(), sessionID)
	return err
}

// ClearSessionDeferred remove a marca de adiada (ao responder/encerrar). Sem erro
// se a sessão não estava adiada.
func (s *Store) ClearSessionDeferred(sessionID string) error {
	_, err := s.db.Exec(`UPDATE sessions SET deferred_at=NULL WHERE id=?`, sessionID)
	return err
}

// ListDeferredSessions devolve as sessões adiadas, mais recentes primeiro.
// O sidebar mostra só as 5 primeiras; aqui não cortamos (cabe ao chamador).
func (s *Store) ListDeferredSessions() ([]DeferredSession, error) {
	rows, err := s.db.Query(`SELECT id, COALESCE(project_id,''), deferred_at
		FROM sessions WHERE deferred_at IS NOT NULL AND status != 'archived'
		ORDER BY deferred_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DeferredSession{}
	for rows.Next() {
		var d DeferredSession
		if err := rows.Scan(&d.SessionID, &d.ProjectID, &d.DeferredAt); err != nil {
			return nil, err
		}
		d.Label = s.SessionLabel(d.SessionID)
		out = append(out, d)
	}
	return out, rows.Err()
}
