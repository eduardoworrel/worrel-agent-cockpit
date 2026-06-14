package store

// Helpers exclusivos de teste — este arquivo só é compilado no binário de
// teste do pacote store, mantendo o código de produção limpo.

// SetSessionTimesForTest envelhece started_at/ended_at em `agoMs`
// milissegundos atrás e marca a sessão como encerrada. Uso: testes.
func (s *Store) SetSessionTimesForTest(id string, agoMs int64) error {
	at := now() - agoMs
	_, err := s.db.Exec(`UPDATE sessions SET status='ended', ended_at=?, started_at=? WHERE id=?`, at, at, id)
	return err
}
