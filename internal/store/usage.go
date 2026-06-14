package store

// SkillStats contém métricas agregadas de uso de uma skill (spec §4.2).
type SkillStats struct {
	TotalUses     int     `json:"total_uses"`
	SuccessCount  int     `json:"success_count"`
	ErrorCount    int     `json:"error_count"`
	EdgeCases     int     `json:"edge_cases"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
	SuccessRate   float64 `json:"success_rate"`
	Trend         string  `json:"trend"` // improving | stable | degrading
	ConsecFail    int     `json:"consec_fail"`
}


// RecordSkillUsageStart registra o início de um uso de skill. Retorna o ID gerado.
func (s *Store) RecordSkillUsageStart(skillID string, sessionID *string, generation int64) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO skill_usage (skill_id, session_id, generation, started_at)
		VALUES (?,?,?,?)`, skillID, sessionID, generation, now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CloseSkillUsage registra o desfecho de um uso.
func (s *Store) CloseSkillUsage(id int64, outcome string, errors int, newEdgeCase bool, durationMs int64) error {
	edge := 0
	if newEdgeCase {
		edge = 1
	}
	_, err := s.db.Exec(`UPDATE skill_usage SET outcome=?, errors=?, new_edge_case=?, duration_ms=?, resolved_at=? WHERE id=?`,
		outcome, errors, edge, durationMs, now(), id)
	return err
}

// CloseSkillUsageBySession fecha o uso aberto mais recente (resolved_at NULL)
// da skill na sessão dada. Usado pelo auto-relato MCP (report_task_completed
// com skill_id). Se sessionID for vazio, fecha o uso aberto mais recente da
// skill em qualquer sessão. No-op silencioso se não houver uso aberto.
func (s *Store) CloseSkillUsageBySession(skillID, sessionID, outcome string, errors int, newEdgeCase bool, durationMs int64) error {
	edge := 0
	if newEdgeCase {
		edge = 1
	}
	q := `UPDATE skill_usage SET outcome=?, errors=?, new_edge_case=?, duration_ms=?, resolved_at=?
		WHERE id = (SELECT id FROM skill_usage WHERE skill_id=? AND resolved_at IS NULL `
	args := []any{outcome, errors, edge, durationMs, now(), skillID}
	if sessionID != "" {
		q += `AND session_id=? `
		args = append(args, sessionID)
	}
	q += `ORDER BY started_at DESC LIMIT 1)`
	_, err := s.db.Exec(q, args...)
	return err
}

// SkillStats retorna estatísticas agregadas para uma skill (spec §4.2):
// usos totais, contagens, duração média, taxa de sucesso, tendência (janela
// móvel) e falhas consecutivas.
func (s *Store) SkillStats(skillID string) (*SkillStats, error) {
	st := &SkillStats{}
	err := s.db.QueryRow(`SELECT
		count(*),
		COALESCE(sum(CASE WHEN outcome='success' THEN 1 ELSE 0 END), 0),
		COALESCE(sum(CASE WHEN outcome='error' THEN 1 ELSE 0 END), 0),
		COALESCE(sum(new_edge_case), 0),
		COALESCE(avg(duration_ms),0)
		FROM skill_usage WHERE skill_id=? AND resolved_at IS NOT NULL`, skillID).
		Scan(&st.TotalUses, &st.SuccessCount, &st.ErrorCount, &st.EdgeCases, &st.AvgDurationMs)
	if err != nil {
		return st, err
	}
	if st.TotalUses > 0 {
		st.SuccessRate = float64(st.SuccessCount) / float64(st.TotalUses)
	}
	st.ConsecFail, _ = s.ConsecutiveFailures(skillID)
	st.Trend, _ = s.skillTrend(skillID)
	return st, nil
}

// skillTrend compara a metade mais recente da janela móvel com a mais antiga.
func (s *Store) skillTrend(skillID string) (string, error) {
	rows, err := s.db.Query(`SELECT outcome FROM skill_usage
		WHERE skill_id=? AND resolved_at IS NOT NULL ORDER BY started_at DESC LIMIT 20`, skillID)
	if err != nil {
		return "stable", err
	}
	defer rows.Close()
	var list []string
	for rows.Next() {
		var o string
		if err := rows.Scan(&o); err != nil {
			return "stable", err
		}
		list = append(list, o)
	}
	if len(list) < 4 {
		return "stable", rows.Err()
	}
	// list[0] = mais recente. Metade da frente = recente; metade de trás = antiga.
	half := len(list) / 2
	var newS, oldS int
	for i, o := range list {
		ok := 0
		if o == "success" {
			ok = 1
		}
		if i < half {
			newS += ok
		} else {
			oldS += ok
		}
	}
	nRate := float64(newS) / float64(half)
	oRate := float64(oldS) / float64(len(list)-half)
	switch {
	case nRate > oRate+0.1:
		return "improving", nil
	case nRate < oRate-0.1:
		return "degrading", nil
	default:
		return "stable", nil
	}
}

// ConsecutiveFailures retorna o número de falhas consecutivas mais recentes.
func (s *Store) ConsecutiveFailures(skillID string) (int, error) {
	rows, err := s.db.Query(`SELECT outcome FROM skill_usage WHERE skill_id=? AND resolved_at IS NOT NULL
		ORDER BY started_at DESC LIMIT 20`, skillID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var outcome string
		if err := rows.Scan(&outcome); err != nil {
			return 0, err
		}
		if outcome == "error" {
			count++
		} else {
			break
		}
	}
	return count, rows.Err()
}

// AbandonOpenUsages fecha como "abandon" os usos em aberto (sem resolved_at).
// Se sessionIDs for fornecido, restringe às sessões dadas (fallback da
// varredura diferida, spec §4.1); senão fecha todos os usos órfãos.
func (s *Store) AbandonOpenUsages(sessionIDs ...string) error {
	if len(sessionIDs) == 0 {
		_, err := s.db.Exec(`UPDATE skill_usage SET outcome='abandon', resolved_at=? WHERE resolved_at IS NULL`, now())
		return err
	}
	for _, id := range sessionIDs {
		if _, err := s.db.Exec(`UPDATE skill_usage SET outcome='abandon', resolved_at=?
			WHERE session_id=? AND resolved_at IS NULL`, now(), id); err != nil {
			return err
		}
	}
	return nil
}
