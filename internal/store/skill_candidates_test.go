package store

import "testing"

func TestSkillCandidateAccumulateAndDedupe(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")

	c, err := s.UpsertSkillCandidate(p.ID, "sig-deploy", "Deploy", `{}`,
		CandidateOccurrence{SessionID: "s1", Signal: "user_steps"})
	if err != nil || c.Occurrences != 1 {
		t.Fatalf("first: occ=%d err=%v", c.Occurrences, err)
	}
	// mesma sessão → não conta de novo
	c, _ = s.UpsertSkillCandidate(p.ID, "sig-deploy", "Deploy", `{}`,
		CandidateOccurrence{SessionID: "s1", Signal: "user_steps"})
	if c.Occurrences != 1 {
		t.Fatalf("dedupe falhou: occ=%d", c.Occurrences)
	}
	// sessão nova → incrementa
	c, _ = s.UpsertSkillCandidate(p.ID, "sig-deploy", "Deploy", `{}`,
		CandidateOccurrence{SessionID: "s2", Signal: "user_steps"})
	if c.Occurrences != 2 {
		t.Fatalf("incremento falhou: occ=%d", c.Occurrences)
	}

	acc, _ := s.ListSkillCandidates(p.ID, "accumulating")
	if len(acc) != 1 {
		t.Fatalf("accumulating=%d", len(acc))
	}
	if err := s.MatureSkillCandidate(c.ID); err != nil {
		t.Fatal(err)
	}
	if m, _ := s.ListSkillCandidates(p.ID, "matured"); len(m) != 1 {
		t.Fatalf("matured=%d", len(m))
	}
}

func TestMarkCandidateExplicit(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	c, _ := s.MarkCandidateExplicit(p.ID, "sig-x", "X", `{}`,
		CandidateOccurrence{SessionID: "s1", Signal: "explicit"})
	if c.ExplicitMark != 1 {
		t.Fatalf("explicit=%d", c.ExplicitMark)
	}
}
