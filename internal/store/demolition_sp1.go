package store

// migrateDemolitionSP1 executa, uma única vez, a tabula rasa do SP1: zera todos
// os dados derivados do motor velho (sugestões/skills/memórias/retro/chat),
// remove os settings de distill e dropa as tabelas de retro/chat (cujo código
// foi deletado). Preserva sessions e transcript_events. Guardada por flag em
// settings — re-rodar é no-op.
func (s *Store) migrateDemolitionSP1() error {
	if s.GetSetting("sp1_demolition_done", "") == "1" {
		return nil
	}
	stmts := []string{
		`DELETE FROM suggestions`,
		`DELETE FROM skills`,
		`DELETE FROM skill_generations`,
		`DELETE FROM skill_usage`,
		`DELETE FROM memory_versions`,
		`DROP TABLE IF EXISTS retro_run_sessions`,
		`DROP TABLE IF EXISTS retro_clusters`,
		`DROP TABLE IF EXISTS retro_runs`,
		`DROP TABLE IF EXISTS chat_messages`,
		`DROP TABLE IF EXISTS chat_threads`,
		`DELETE FROM settings WHERE key IN
			('headless_adapter','health_consec_failures','health_min_success_rate',
			 'auto_daily_cap','prompt.skill','prompt.memory','prompt.scope')`,
		`UPDATE sessions SET analyzed_at=NULL`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return s.SetSetting("sp1_demolition_done", "1")
}

// openMigrateDemolitionSP1 is called from Open: it only runs the demolition if
// the legacy retro/chat tables are present (i.e. the DB pre-dates SP1). Fresh
// installs skip it entirely so the flag is not pre-consumed.
func (s *Store) openMigrateDemolitionSP1() error {
	var n int
	if err := s.db.QueryRow(
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='retro_runs'`,
	).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return nil // fresh install — nothing to demolish
	}
	return s.migrateDemolitionSP1()
}
