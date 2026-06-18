package store

// --- supressão de segredos (critério 9; valor cru NUNCA armazenado) ---

func (s *Store) IsSecretSuppressed(hash string) bool {
	var n int
	_ = s.db.QueryRow(`SELECT count(*) FROM secret_suppressions WHERE hash=?`, hash).Scan(&n)
	return n > 0
}

func (s *Store) SuppressSecret(hash string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO secret_suppressions (hash, created_at) VALUES (?,?)`, hash, now())
	return err
}
