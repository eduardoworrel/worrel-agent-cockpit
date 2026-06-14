package store

func (s *Store) MigrateSkillsToLineage() error {
	rows, err := s.db.Query(`SELECT id, content FROM skills`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type skill struct{ id, content string }
	var skills []skill
	for rows.Next() {
		var sk skill
		if err := rows.Scan(&sk.id, &sk.content); err != nil {
			return err
		}
		skills = append(skills, sk)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, sk := range skills {
		if err := s.SeedGeneration(sk.id, sk.content); err != nil {
			return err
		}
	}
	return nil
}
