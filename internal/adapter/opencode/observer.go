package opencode

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	_ "modernc.org/sqlite"
)

// dbPath returns the configured path or the default opencode DB location.
func (a *Adapter) dbPath() string {
	if a.DBPath != "" {
		return a.DBPath
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "opencode", "opencode.db")
}

// openRO abre o db do opencode em modo somente-leitura para nunca interferir.
func (a *Adapter) openRO() (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+a.dbPath()+"?mode=ro&immutable=1&_pragma=busy_timeout(2000)")
	if err != nil {
		return nil, err
	}
	// Allow multiple connections so nested queries (joinParts) don't deadlock.
	db.SetMaxOpenConns(5)
	return db, nil
}

// DiscoverSessionsProgress devolve as sessões do opencode reportando progresso.
// Como a leitura é via SQLite (rápida), o progresso é simples: 0/total no início
// e total/total no fim. onProgress pode ser nil.
func (a *Adapter) DiscoverSessionsProgress(since time.Time, onProgress func(done, total int)) ([]adapter.ExternalSession, error) {
	out, err := a.DiscoverSessions(since)
	if err != nil {
		return nil, err
	}
	if onProgress != nil {
		onProgress(0, len(out))
		onProgress(len(out), len(out))
	}
	return out, nil
}

func (a *Adapter) DiscoverSessions(since time.Time) ([]adapter.ExternalSession, error) {
	db, err := a.openRO()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	q := `SELECT id, COALESCE(directory,''), COALESCE(title,''), time_created, time_updated FROM session`
	args := []any{}
	if !since.IsZero() {
		q += ` WHERE time_updated >= ?`
		args = append(args, since.UnixMilli())
	}
	q += ` ORDER BY time_updated DESC`
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []adapter.ExternalSession
	for rows.Next() {
		var id, dir, title string
		var created, updated int64
		if err := rows.Scan(&id, &dir, &title, &created, &updated); err != nil {
			return nil, err
		}
		out = append(out, adapter.ExternalSession{
			Adapter: "opencode", ExternalRef: id, Dir: dir, Title: title,
			StartedAt: time.UnixMilli(created), UpdatedAt: time.UnixMilli(updated),
		})
	}
	return out, rows.Err()
}

type ocMessage struct {
	Role   string `json:"role"`
	Tokens struct {
		Input  int64 `json:"input"`
		Output int64 `json:"output"`
	} `json:"tokens"`
	Time struct {
		Created int64 `json:"created"`
	} `json:"time"`
}

type ocPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (a *Adapter) ReadTranscript(ref adapter.SessionRef) ([]adapter.TranscriptEvent, error) {
	db, err := a.openRO()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT id, data FROM message WHERE session_id=? ORDER BY time_created, id`, ref.ExternalRef)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []adapter.TranscriptEvent
	for rows.Next() {
		var mid, data string
		if err := rows.Scan(&mid, &data); err != nil {
			return nil, err
		}
		var m ocMessage
		if json.Unmarshal([]byte(data), &m) != nil {
			continue
		}
		text := a.joinParts(db, mid)
		if strings.TrimSpace(text) == "" {
			continue
		}
		out = append(out, adapter.TranscriptEvent{
			Role: m.Role, Kind: "text", Content: text,
			TokensIn: m.Tokens.Input, TokensOut: m.Tokens.Output,
			CreatedAt: m.Time.Created,
		})
	}
	return out, rows.Err()
}

func (a *Adapter) joinParts(db *sql.DB, messageID string) string {
	rows, err := db.Query(`SELECT data FROM part WHERE message_id=? ORDER BY id`, messageID)
	if err != nil {
		return ""
	}
	defer rows.Close()
	var parts []string
	for rows.Next() {
		var data string
		if rows.Scan(&data) != nil {
			continue
		}
		var p ocPart
		if json.Unmarshal([]byte(data), &p) != nil {
			continue
		}
		if (p.Type == "text" || p.Type == "reasoning") && p.Text != "" {
			parts = append(parts, p.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}
