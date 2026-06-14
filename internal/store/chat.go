package store

import (
	"github.com/google/uuid"
)

// ChatThread é uma conversa do Chat de Destilação. Scope é JSON livre
// ({project_id?, cluster?, window_days?, clis[]?}) que delimita as sessões
// recuperadas como contexto. Provider/Model definem o LLM headless usado.
type ChatThread struct {
	ID        string `json:"id"`
	Scope     string `json:"scope"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	Title     string `json:"title"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// ChatMessage é uma mensagem (user|assistant) de um thread. Sources é JSON com
// os refs das sessões usadas como contexto na resposta do assistant.
type ChatMessage struct {
	ThreadID  string `json:"thread_id"`
	Seq       int64  `json:"seq"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	Sources   string `json:"sources"`
	CreatedAt int64  `json:"created_at"`
}

func (s *Store) CreateChatThread(scope, provider, model, title string) (*ChatThread, error) {
	if scope == "" {
		scope = "{}"
	}
	t := &ChatThread{
		ID:        uuid.NewString(),
		Scope:     scope,
		Provider:  provider,
		Model:     model,
		Title:     title,
		CreatedAt: now(),
		UpdatedAt: now(),
	}
	_, err := s.db.Exec(`INSERT INTO chat_threads
		(id, scope, provider, model, title, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?)`,
		t.ID, t.Scope, t.Provider, t.Model, t.Title, t.CreatedAt, t.UpdatedAt)
	return t, err
}

const chatThreadCols = `SELECT id, scope, provider, model, title, created_at, updated_at FROM chat_threads`

func scanChatThread(r rowScanner) (*ChatThread, error) {
	t := &ChatThread{}
	err := r.Scan(&t.ID, &t.Scope, &t.Provider, &t.Model, &t.Title, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func (s *Store) GetChatThread(id string) (*ChatThread, error) {
	return scanChatThread(s.db.QueryRow(chatThreadCols+` WHERE id=?`, id))
}

// ListChatThreads devolve as conversas mais recentes primeiro.
func (s *Store) ListChatThreads() ([]*ChatThread, error) {
	rows, err := s.db.Query(chatThreadCols + ` ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*ChatThread{}
	for rows.Next() {
		t, err := scanChatThread(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// AppendChatMessage adiciona uma mensagem ao thread (seq auto-incremental) e
// avança updated_at do thread. sourcesJSON vazio vira "[]".
func (s *Store) AppendChatMessage(threadID, role, content, sourcesJSON string) (*ChatMessage, error) {
	if sourcesJSON == "" {
		sourcesJSON = "[]"
	}
	t := now()
	var seq int64
	err := s.db.QueryRow(
		`SELECT COALESCE(MAX(seq),0)+1 FROM chat_messages WHERE thread_id=?`, threadID).Scan(&seq)
	if err != nil {
		return nil, err
	}
	_, err = s.db.Exec(`INSERT INTO chat_messages
		(thread_id, seq, role, content, sources, created_at)
		VALUES (?,?,?,?,?,?)`,
		threadID, seq, role, content, sourcesJSON, t)
	if err != nil {
		return nil, err
	}
	_, _ = s.db.Exec(`UPDATE chat_threads SET updated_at=? WHERE id=?`, t, threadID)
	return &ChatMessage{
		ThreadID:  threadID,
		Seq:       seq,
		Role:      role,
		Content:   content,
		Sources:   sourcesJSON,
		CreatedAt: t,
	}, nil
}

// ListChatMessages devolve as mensagens do thread em ordem cronológica.
func (s *Store) ListChatMessages(threadID string) ([]*ChatMessage, error) {
	rows, err := s.db.Query(`SELECT thread_id, seq, role, content, sources, created_at
		FROM chat_messages WHERE thread_id=? ORDER BY seq`, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*ChatMessage{}
	for rows.Next() {
		m := &ChatMessage{}
		if err := rows.Scan(&m.ThreadID, &m.Seq, &m.Role, &m.Content, &m.Sources, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
