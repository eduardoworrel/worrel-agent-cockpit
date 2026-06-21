CREATE TABLE IF NOT EXISTS projects (
  id TEXT PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS project_dirs (
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  dir TEXT NOT NULL,
  PRIMARY KEY (project_id, dir)
);
CREATE TABLE IF NOT EXISTS memory_versions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  content TEXT NOT NULL,
  note TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS skills (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  slug TEXT NOT NULL,
  name TEXT NOT NULL,
  content TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  UNIQUE (project_id, slug)
);
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  project_id TEXT REFERENCES projects(id) ON DELETE SET NULL,
  adapter TEXT NOT NULL,
  external_ref TEXT,
  mode TEXT NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  continues TEXT REFERENCES sessions(id),
  mcp_token TEXT,
  started_at INTEGER NOT NULL,
  ended_at INTEGER,
  analyzed_at INTEGER,
  context_used INTEGER NOT NULL DEFAULT 0,
  context_limit INTEGER NOT NULL DEFAULT 0,
  summary TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS transcript_events (
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  seq INTEGER NOT NULL,
  role TEXT NOT NULL,
  kind TEXT NOT NULL,
  content TEXT NOT NULL,
  tokens_in INTEGER NOT NULL DEFAULT 0,
  tokens_out INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (session_id, seq)
);
CREATE TABLE IF NOT EXISTS suggestions (
  id TEXT PRIMARY KEY,
  project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
  session_id TEXT REFERENCES sessions(id) ON DELETE SET NULL,
  skill_id TEXT REFERENCES skills(id) ON DELETE SET NULL,
  type TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  title TEXT NOT NULL,
  payload TEXT NOT NULL DEFAULT '{}',
  evidence TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  resolved_at INTEGER
);
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS secrets (
  id TEXT PRIMARY KEY,
  project_id TEXT REFERENCES projects(id) ON DELETE CASCADE, -- NULL = global
  name TEXT NOT NULL,
  mode TEXT NOT NULL,            -- value | recipe
  ciphertext BLOB,              -- preenchido no modo value
  recipe TEXT,                  -- preenchido no modo recipe
  policy TEXT NOT NULL DEFAULT 'per_access', -- always | per_session | per_access
  injectable INTEGER NOT NULL DEFAULT 0,     -- 0/1
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  UNIQUE (project_id, name)
);
-- Auditoria é PERMANENTE (spec §11): NÃO cascateia ao apagar o segredo.
-- Mantém o nome denormalizado para preservar o registro após a remoção.
CREATE TABLE IF NOT EXISTS secret_audit (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  secret_id TEXT REFERENCES secrets(id) ON DELETE SET NULL,
  secret_name TEXT NOT NULL,
  session_id TEXT,
  project_id TEXT,
  action TEXT NOT NULL,         -- requested | granted | denied | expired
  detail TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);

-- NOTA: colunas adicionadas após o release inicial entram via
-- migrateAddColumns() em store.go (ALTER TABLE idempotente por pragma_table_info),
-- pois ALTER não suporta IF NOT EXISTS. Não edite o meio deste arquivo.
CREATE TABLE IF NOT EXISTS skill_generations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  skill_id TEXT NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
  generation INTEGER NOT NULL,
  evolution_type TEXT NOT NULL,
  parent_skill_ids TEXT NOT NULL DEFAULT '[]',
  diff TEXT NOT NULL DEFAULT '',
  snapshot TEXT NOT NULL,
  change_summary TEXT NOT NULL DEFAULT '',
  evidence TEXT NOT NULL DEFAULT '',
  authorship TEXT NOT NULL DEFAULT 'human',
  created_at INTEGER NOT NULL,
  UNIQUE (skill_id, generation)
);
CREATE TABLE IF NOT EXISTS skill_usage (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  skill_id TEXT NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
  session_id TEXT REFERENCES sessions(id) ON DELETE SET NULL,
  generation INTEGER NOT NULL DEFAULT 1,
  started_at INTEGER NOT NULL,
  outcome TEXT NOT NULL DEFAULT '',
  errors INTEGER NOT NULL DEFAULT 0,
  new_edge_case INTEGER NOT NULL DEFAULT 0,
  duration_ms INTEGER NOT NULL DEFAULT 0,
  resolved_at INTEGER
);
CREATE INDEX IF NOT EXISTS idx_skill_usage_skill ON skill_usage(skill_id, started_at);
CREATE INDEX IF NOT EXISTS idx_skill_gen_skill ON skill_generations(skill_id, generation);

CREATE TABLE IF NOT EXISTS secret_suppressions (
  hash TEXT PRIMARY KEY,                         -- sha256 do valor cru; valor NUNCA armazenado
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS memory_entries (
  id            TEXT PRIMARY KEY,
  project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  content       TEXT NOT NULL,
  category      TEXT NOT NULL DEFAULT 'gotcha',
  evidence      TEXT NOT NULL DEFAULT '',
  status        TEXT NOT NULL DEFAULT 'active',
  superseded_by TEXT NOT NULL DEFAULT '',
  created_at    INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS agents (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  slug       TEXT NOT NULL DEFAULT '',
  name       TEXT NOT NULL,
  persona    TEXT NOT NULL,
  evidence   TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS engine_config (
  engine_id  TEXT NOT NULL,
  scope_key  TEXT NOT NULL,            -- '' = global; senão project_id
  key        TEXT NOT NULL,
  value      TEXT NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (engine_id, scope_key, key)
);
