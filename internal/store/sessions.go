package store

import (
	"database/sql"

	"github.com/google/uuid"
)

type Session struct {
	ID           string  `json:"id"`
	ProjectID    string  `json:"project_id"`
	Adapter      string  `json:"adapter"`
	ExternalRef  *string `json:"external_ref"`
	Mode         string  `json:"mode"` // wrapper | observed
	Title        string  `json:"title"`
	Status       string  `json:"status"` // active | ended | archived
	Continues    *string `json:"continues"`
	MCPToken     *string `json:"-"`
	StartedAt    int64   `json:"started_at"`
	EndedAt      *int64  `json:"ended_at"`
	AnalyzedAt   *int64  `json:"analyzed_at"`
	ContextUsed  int64   `json:"context_used"`
	ContextLimit int64   `json:"context_limit"`
	Summary          string `json:"summary"`
	TranscriptPruned bool   `json:"transcript_pruned"`
	WorkspaceDir     string `json:"workspace_dir"`
	SourceDir        string `json:"source_dir"`
}

type TranscriptEvent struct {
	SessionID string `json:"session_id"`
	Seq       int64  `json:"seq"`
	Role      string `json:"role"`
	Kind      string `json:"kind"`
	Content   string `json:"content"`
	TokensIn  int64  `json:"tokens_in"`
	TokensOut int64  `json:"tokens_out"`
	CreatedAt int64  `json:"created_at"`
}

func (s *Store) CreateSession(sess *Session) (*Session, error) {
	if sess.ID == "" {
		sess.ID = uuid.NewString()
	}
	if sess.Status == "" {
		sess.Status = "active"
	}
	// Sessões in-app (wrapper) são spawnadas com --session-id = sess.ID, logo o
	// ref externo do CLI é o próprio id. Marcá-lo aqui (a) deduplica o importer
	// — que casa por external_ref e deixa de criar uma gêmea "observed" — e
	// (b) permite ao handoff resolver o .jsonl da sessão para ler o transcript.
	if sess.Mode == "wrapper" && sess.ExternalRef == nil {
		sess.ExternalRef = &sess.ID
	}
	sess.StartedAt = now()
	_, err := s.db.Exec(`INSERT INTO sessions
		(id, project_id, adapter, external_ref, mode, title, status, continues, mcp_token, started_at, workspace_dir, source_dir)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		sess.ID, nullable(sess.ProjectID), sess.Adapter, sess.ExternalRef, sess.Mode,
		sess.Title, sess.Status, sess.Continues, sess.MCPToken, sess.StartedAt, sess.WorkspaceDir, sess.SourceDir)
	return sess, err
}

func (s *Store) GetSession(id string) (*Session, error) {
	return scanSession(s.db.QueryRow(sessionCols+` WHERE id=?`, id))
}

func (s *Store) SessionByMCPToken(token string) (*Session, error) {
	return scanSession(s.db.QueryRow(sessionCols+` WHERE mcp_token=?`, token))
}

const sessionCols = `SELECT id, COALESCE(project_id,''), adapter, external_ref, mode, title, status,
	continues, mcp_token, started_at, ended_at, analyzed_at, context_used, context_limit, summary,
	transcript_pruned, COALESCE(workspace_dir,''), COALESCE(source_dir,'')
	FROM sessions`

func scanSession(r rowScanner) (*Session, error) {
	x := &Session{}
	err := r.Scan(&x.ID, &x.ProjectID, &x.Adapter, &x.ExternalRef, &x.Mode, &x.Title, &x.Status,
		&x.Continues, &x.MCPToken, &x.StartedAt, &x.EndedAt, &x.AnalyzedAt,
		&x.ContextUsed, &x.ContextLimit, &x.Summary, &x.TranscriptPruned, &x.WorkspaceDir, &x.SourceDir)
	return x, err
}

// ListSessions com projectID vazio lista todas (mais recentes primeiro).
func (s *Store) ListSessions(projectID string) ([]*Session, error) {
	q := sessionCols
	args := []any{}
	if projectID != "" {
		q += ` WHERE project_id=?`
		args = append(args, projectID)
	}
	q += ` ORDER BY started_at DESC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Session{}
	for rows.Next() {
		x, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) EndSession(id string) error {
	result, err := s.db.Exec(`UPDATE sessions SET status='ended', ended_at=? WHERE id=?`, now(), id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SessionIDByExternalRef devolve o id da sessão observada com o external_ref dado
// (vazio se inexistente). Usado pela análise retroativa para mapear sessões importadas.
func (s *Store) SessionIDByExternalRef(ref string) (string, error) {
	var id string
	err := s.db.QueryRow(`SELECT id FROM sessions WHERE external_ref=? LIMIT 1`, ref).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}

func (s *Store) SetSessionMCPToken(id, token string) error {
	_, err := s.db.Exec(`UPDATE sessions SET mcp_token=? WHERE id=?`, token, id)
	return err
}

func (s *Store) UpdateSessionContext(id string, used, limit int64) error {
	result, err := s.db.Exec(`UPDATE sessions SET context_used=?, context_limit=? WHERE id=?`, used, limit, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) AppendTranscriptEvent(sessionID, role, kind, content string, tokIn, tokOut int64) error {
	_, err := s.db.Exec(`INSERT INTO transcript_events
		(session_id, seq, role, kind, content, tokens_in, tokens_out, created_at)
		VALUES (?, COALESCE((SELECT MAX(seq) FROM transcript_events WHERE session_id=?),0)+1, ?,?,?,?,?,?)`,
		sessionID, sessionID, role, kind, content, tokIn, tokOut, now())
	return err
}

func (s *Store) ListTranscriptEvents(sessionID string) ([]*TranscriptEvent, error) {
	rows, err := s.db.Query(`SELECT session_id, seq, role, kind, content, tokens_in, tokens_out, created_at
		FROM transcript_events WHERE session_id=? ORDER BY seq`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*TranscriptEvent{}
	for rows.Next() {
		e := &TranscriptEvent{}
		if err := rows.Scan(&e.SessionID, &e.Seq, &e.Role, &e.Kind, &e.Content,
			&e.TokensIn, &e.TokensOut, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ExpiredSessionIDs devolve ids de sessões cujo transcript já passou da
// janela de retenção (cutoff = limite em epoch-ms) e que ainda não foram podadas.
func (s *Store) ExpiredSessionIDs(cutoff int64) ([]string, error) {
	rows, err := s.db.Query(`SELECT id FROM sessions
		WHERE transcript_pruned = 0
		  AND COALESCE(ended_at, started_at) < ?`, cutoff)
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
	return ids, rows.Err()
}

// PruneSessionTranscript apaga os eventos brutos de transcript de uma sessão
// e marca transcript_pruned=1. NÃO toca em metadados da sessão nem em
// suggestions/evidências/auditoria — todos permanentes por construção (spec §11).
func (s *Store) PruneSessionTranscript(sessionID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM transcript_events WHERE session_id=?`, sessionID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE sessions SET transcript_pruned=1 WHERE id=?`, sessionID); err != nil {
		return err
	}
	return tx.Commit()
}

// SetSessionTitleIfEmpty preenche o título da sessão apenas quando ainda vazio,
// devolvendo se houve gravação. Idempotente: títulos já definidos (sessões
// observadas, ou um título derivado anterior) nunca são sobrescritos — o
// tracker do wrapper chama isto a cada poll e para de tentar quando ok=false.
func (s *Store) SetSessionTitleIfEmpty(id, title string) (bool, error) {
	res, err := s.db.Exec(`UPDATE sessions SET title=? WHERE id=? AND COALESCE(title,'')=''`, title, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// SetSessionSummary grava o resumo estruturado de handoff.
func (s *Store) SetSessionSummary(id, summary string) error {
	_, err := s.db.Exec(`UPDATE sessions SET summary=? WHERE id=?`, summary, id)
	return err
}

// ArchiveSession marca a sessão antiga como arquivada (handoff).
func (s *Store) ArchiveSession(id string) error {
	_, err := s.db.Exec(`UPDATE sessions SET status='archived', ended_at=COALESCE(ended_at, ?) WHERE id=?`, now(), id)
	return err
}

// ContinuedBy devolve o id da sessão que continua `id` (ou nil).
func (s *Store) ContinuedBy(id string) (*string, error) {
	var by string
	err := s.db.QueryRow(`SELECT id FROM sessions WHERE continues=? LIMIT 1`, id).Scan(&by)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &by, nil
}

// ClassifySession atrela uma sessão (tipicamente não-classificada) a um projeto.
func (s *Store) ClassifySession(sessionID, projectID string) error {
	res, err := s.db.Exec(`UPDATE sessions SET project_id=? WHERE id=?`, projectID, sessionID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// PromoteSessionToProject cria um projeto a partir de uma sessão não-classificada
// e atrela a sessão a ele. Retorna o projeto criado.
// TODO: operação não é atômica (create + classify em duas queries) — aceitável por ora, flagged em review.
func (s *Store) PromoteSessionToProject(sessionID, name, description string) (*Project, error) {
	p, err := s.CreateProject(name, description)
	if err != nil {
		return nil, err
	}
	if err := s.ClassifySession(sessionID, p.ID); err != nil {
		return nil, err
	}
	return p, nil
}

// ListActiveWrapperSessions lista sessões wrapper ativas (para a faixa de abas).
func (s *Store) ListActiveWrapperSessions() ([]*Session, error) {
	rows, err := s.db.Query(sessionCols + ` WHERE mode='wrapper' AND status='active' ORDER BY started_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Session{}
	for rows.Next() {
		x, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// EndOrphanedWrapperSessions encerra, no boot, toda sessão wrapper ainda marcada
// como active. Um PTY de wrapper vive apenas no processo que o spawnou: após um
// restart/reinstalação do servidor o mapa em memória nasce vazio, então qualquer
// sessão active no banco é órfã (não há terminal vivo por trás). Sem isto elas
// reaparecem na faixa de abas e ao clicar o usuário só encontra uma sessão morta
// que precisa re-encerrar à mão. Devolve quantas foram reconciliadas.
func (s *Store) EndOrphanedWrapperSessions() (int64, error) {
	res, err := s.db.Exec(`UPDATE sessions SET status='ended', ended_at=COALESCE(ended_at, ?)
		WHERE mode='wrapper' AND status='active'`, now())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SetSessionWorkspaceDir persiste o workspace resolvido na sessão.
func (s *Store) SetSessionWorkspaceDir(id, dir string) error {
	_, err := s.db.Exec(`UPDATE sessions SET workspace_dir=? WHERE id=?`, dir, id)
	return err
}
