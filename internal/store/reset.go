package store

// DataDir devolve o diretório de dados configurado (para o reset limpar mirror/workspaces).
func (s *Store) DataDir() string { return s.dataDir }

// resetTables são TODAS as tabelas de dados, em ordem filho→pai para não violar
// FKs ao esvaziar. É um "factory reset": projetos, memórias, skills, sugestões,
// sessões, segredos, retroativa, chat e settings voltam ao zero. O schema é
// preservado; o Keychain (chave-mestra do SO) NÃO é tocado.
var resetTables = []string{
	"skill_usage", "skill_generations",
	"secret_audit", "secret_suppressions", "secrets",
	"suggestions", "transcript_events", "sessions",
	"memory_versions", "project_dirs", "skills", "projects",
	"settings",
}

// ResetAll esvazia todas as tabelas de dados numa transação. Idempotente.
func (s *Store) ResetAll() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range resetTables {
		if _, err := tx.Exec("DELETE FROM " + t); err != nil {
			return err
		}
	}
	return tx.Commit()
}
