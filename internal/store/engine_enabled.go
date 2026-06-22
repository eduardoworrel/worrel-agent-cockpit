package store

// EngineEnabled resolve o toggle __enabled de um motor de borda (summary,
// interpret) na ordem: override de sessão ("session:<id>") ⊕ global ("") ⊕
// defaultOn. sessionID vazio ignora a camada de sessão (toggles só-globais).
func (s *Store) EngineEnabled(engineID, sessionID string, defaultOn bool) bool {
	if sessionID != "" {
		if m, err := s.GetEngineConfig(engineID, "session:"+sessionID); err == nil {
			if v, ok := m["__enabled"]; ok {
				return v == "true"
			}
		}
	}
	if m, err := s.GetEngineConfig(engineID, ""); err == nil {
		if v, ok := m["__enabled"]; ok {
			return v == "true"
		}
	}
	return defaultOn
}
