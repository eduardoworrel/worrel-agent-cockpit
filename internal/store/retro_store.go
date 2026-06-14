package store

import (
	"database/sql"

	"github.com/google/uuid"
)

type RetroRun struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	Depth         string `json:"depth"`
	Scope         string `json:"scope"`
	BudgetPerHour int64  `json:"budget_per_hour"`
	BudgetTotal   int64  `json:"budget_total"`
	LLMCalls      int64  `json:"llm_calls"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

func (s *Store) CreateRetroRun(r *RetroRun) (*RetroRun, error) {
	r.ID = uuid.NewString()
	if r.Status == "" {
		r.Status = "inventoried"
	}
	if r.Depth == "" {
		r.Depth = "completa"
	}
	if r.Scope == "" {
		r.Scope = "{}"
	}
	r.CreatedAt = now()
	r.UpdatedAt = now()
	_, err := s.db.Exec(`INSERT INTO retro_runs
		(id,status,depth,scope,budget_per_hour,budget_total,llm_calls,created_at,updated_at)
		VALUES (?,?,?,?,?,?,0,?,?)`,
		r.ID, r.Status, r.Depth, r.Scope, r.BudgetPerHour, r.BudgetTotal, r.CreatedAt, r.UpdatedAt)
	return r, err
}

const retroRunCols = `SELECT id,status,depth,scope,budget_per_hour,budget_total,llm_calls,created_at,updated_at FROM retro_runs`

func scanRetroRun(r rowScanner) (*RetroRun, error) {
	x := &RetroRun{}
	err := r.Scan(&x.ID, &x.Status, &x.Depth, &x.Scope, &x.BudgetPerHour, &x.BudgetTotal,
		&x.LLMCalls, &x.CreatedAt, &x.UpdatedAt)
	return x, err
}

func (s *Store) GetRetroRun(id string) (*RetroRun, error) {
	return scanRetroRun(s.db.QueryRow(retroRunCols + ` WHERE id=?`, id))
}

func (s *Store) ListRetroRuns() ([]*RetroRun, error) {
	rows, err := s.db.Query(retroRunCols + ` ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*RetroRun{}
	for rows.Next() {
		x, err := scanRetroRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) SetRetroRunStatus(id, status string) error {
	return s.touchRun(`UPDATE retro_runs SET status=?, updated_at=? WHERE id=?`, status, now(), id)
}

func (s *Store) SetRetroRunScope(id, scope, depth string, perHour, total int64) error {
	return s.touchRun(`UPDATE retro_runs SET scope=?, depth=?, budget_per_hour=?, budget_total=?, updated_at=? WHERE id=?`,
		scope, depth, perHour, total, now(), id)
}

func (s *Store) IncrRunLLMCalls(id string, by int64) error {
	return s.touchRun(`UPDATE retro_runs SET llm_calls=llm_calls+?, updated_at=? WHERE id=?`, by, now(), id)
}

func (s *Store) touchRun(q string, args ...any) error {
	res, err := s.db.Exec(q, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) AddRunSession(runID, sessionID, projectID string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO retro_run_sessions (run_id,session_id,project_id,state)
		VALUES (?,?,?, 'pending')`, runID, sessionID, nullable(projectID))
	return err
}

func (s *Store) SetRunSessionProject(runID, sessionID, projectID string) error {
	_, err := s.db.Exec(`UPDATE retro_run_sessions SET project_id=? WHERE run_id=? AND session_id=?`,
		nullable(projectID), runID, sessionID)
	return err
}

func (s *Store) MarkRunSessionDone(runID, sessionID string) error {
	_, err := s.db.Exec(`UPDATE retro_run_sessions SET state='done', processed_at=? WHERE run_id=? AND session_id=?`,
		now(), runID, sessionID)
	return err
}

// PendingRunSessions devolve session_id de sessões ainda não processadas (base do critério 4).
func (s *Store) PendingRunSessions(runID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT session_id FROM retro_run_sessions
		WHERE run_id=? AND state='pending' ORDER BY session_id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// PendingRunSessionsByProject agrupa as pendentes (com project_id já definido) por projeto.
func (s *Store) PendingRunSessionsByProject(runID string) (map[string][]string, error) {
	rows, err := s.db.Query(`SELECT COALESCE(project_id,''), session_id FROM retro_run_sessions
		WHERE run_id=? AND state='pending' AND project_id IS NOT NULL ORDER BY project_id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var pid, sid string
		if err := rows.Scan(&pid, &sid); err != nil {
			return nil, err
		}
		out[pid] = append(out[pid], sid)
	}
	return out, rows.Err()
}
