package store

import (
	"database/sql"
	_ "embed"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations.sql
var migrations string

type Store struct {
	db      *sql.DB
	dataDir string
}

func (s *Store) SetDataDir(d string) { s.dataDir = d }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(migrations); err != nil {
		db.Close()
		return nil, err
	}
	if err := migrateAddColumns(db); err != nil {
		db.Close()
		return nil, err
	}
	st := &Store{db: db}
	if err := st.MigrateSkillsToLineage(); err != nil {
		db.Close()
		return nil, err
	}
	if err := st.openMigrateDemolitionSP1(); err != nil {
		db.Close()
		return nil, err
	}
	return st, nil
}

func (s *Store) Close() error { return s.db.Close() }

// DB devolve o handle bruto p/ consultas auxiliares (somente leitura recomendada).
func (s *Store) DB() *sql.DB { return s.db }

func now() int64 { return time.Now().UnixMilli() }

// migrateAddColumns aplica ALTER TABLE idempotentes que não cabem em
// migrations.sql (que usa só CREATE ... IF NOT EXISTS). Cada entrada é
// aplicada apenas se a coluna ainda não existir — seguro para re-rodar e
// para execução concorrente com outras fases (append-only nesta lista).
func migrateAddColumns(db *sql.DB) error {
	type addCol struct{ table, column, ddl string }
	wanted := []addCol{
		{"sessions", "transcript_pruned",
			`ALTER TABLE sessions ADD COLUMN transcript_pruned INTEGER NOT NULL DEFAULT 0`},
		{"skills", "active_generation",
			`ALTER TABLE skills ADD COLUMN active_generation INTEGER NOT NULL DEFAULT 1`},
		{"skills", "evolution_policy",
			`ALTER TABLE skills ADD COLUMN evolution_policy TEXT NOT NULL DEFAULT 'manual'`},
		{"skills", "origin",
			`ALTER TABLE skills ADD COLUMN origin TEXT NOT NULL DEFAULT 'learned'`},
		{"suggestions", "evolution_type",
			`ALTER TABLE suggestions ADD COLUMN evolution_type TEXT NOT NULL DEFAULT ''`},
		{"suggestions", "authorship",
			`ALTER TABLE suggestions ADD COLUMN authorship TEXT NOT NULL DEFAULT ''`},
		{"suggestions", "origin",
			`ALTER TABLE suggestions ADD COLUMN origin TEXT NOT NULL DEFAULT 'incremental'`},
		{"projects", "workspace_dir",
			`ALTER TABLE projects ADD COLUMN workspace_dir TEXT NOT NULL DEFAULT ''`},
		{"sessions", "workspace_dir",
			`ALTER TABLE sessions ADD COLUMN workspace_dir TEXT NOT NULL DEFAULT ''`},
		{"sessions", "source_dir",
			`ALTER TABLE sessions ADD COLUMN source_dir TEXT NOT NULL DEFAULT ''`},
		// motivo do encerramento de uma sessão wrapper: exit code + cauda do
		// stderr do CLI capturados em onExit. Vazio = encerrada sem detalhe
		// (boot reconciliation, kill manual, ou sessões antigas/observed).
		{"sessions", "end_reason",
			`ALTER TABLE sessions ADD COLUMN end_reason TEXT NOT NULL DEFAULT ''`},
		// metadata JSON da skill: {kind:"pipeline", steps:[{skill_id,note,inputs,credentials}]}
		// para skills compostas (pipelines). '{}' para skills normais.
		{"skills", "metadata",
			`ALTER TABLE skills ADD COLUMN metadata TEXT NOT NULL DEFAULT '{}'`},
		// payload JSON por evento de transcript: dados estruturados de ferramenta
		// (tool_use {name,input}; tool_result {output,is_error}). '' para eventos
		// de texto e para adapters sem captura rica.
		{"transcript_events", "payload",
			`ALTER TABLE transcript_events ADD COLUMN payload TEXT NOT NULL DEFAULT ''`},
		{"skills", "structured",
			`ALTER TABLE skills ADD COLUMN structured TEXT NOT NULL DEFAULT ''`},
		{"agents", "active_generation",
			`ALTER TABLE agents ADD COLUMN active_generation INTEGER NOT NULL DEFAULT 1`},
	}
	for _, w := range wanted {
		var n int
		if err := db.QueryRow(
			`SELECT count(*) FROM pragma_table_info(?) WHERE name=?`,
			w.table, w.column).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			if _, err := db.Exec(w.ddl); err != nil {
				return err
			}
		}
	}
	return nil
}
