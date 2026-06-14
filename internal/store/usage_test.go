package store

import "testing"

func TestSkillUsageLifecycle(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sk, _ := s.CreateSkill(p.ID, "Deploy", "# v1")

	uid, err := s.RecordSkillUsageStart(sk.ID, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if uid == 0 {
		t.Fatal("usage id zero")
	}

	if err := s.CloseSkillUsage(uid, "success", 0, false, 120); err != nil {
		t.Fatal(err)
	}

	stats, err := s.SkillStats(sk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalUses != 1 || stats.SuccessCount != 1 || stats.ErrorCount != 0 {
		t.Fatalf("stats: %+v", stats)
	}
}

func TestCloseSkillUsageBySession(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sk, _ := s.CreateSkill(p.ID, "Deploy", "# v1")
	sess, _ := s.CreateSession(&Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})

	if _, err := s.RecordSkillUsageStart(sk.ID, &sess.ID, 1); err != nil {
		t.Fatal(err)
	}
	// Fecha por skill+sessão (caminho do auto-relato MCP).
	if err := s.CloseSkillUsageBySession(sk.ID, sess.ID, "success", 0, false, 0); err != nil {
		t.Fatal(err)
	}
	stats, _ := s.SkillStats(sk.ID)
	if stats.TotalUses != 1 || stats.SuccessCount != 1 {
		t.Fatalf("stats: %+v", stats)
	}
	// Segundo fechamento é no-op (não há uso aberto).
	if err := s.CloseSkillUsageBySession(sk.ID, sess.ID, "error", 1, false, 0); err != nil {
		t.Fatal(err)
	}
	stats, _ = s.SkillStats(sk.ID)
	if stats.TotalUses != 1 {
		t.Fatalf("fechamento duplicado alterou stats: %+v", stats)
	}
}

func TestAbandonOpenUsagesBySession(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sk, _ := s.CreateSkill(p.ID, "Deploy", "# v1")
	sess, _ := s.CreateSession(&Session{ProjectID: p.ID, Adapter: "x", Mode: "observed"})
	_, _ = s.RecordSkillUsageStart(sk.ID, &sess.ID, 1)
	if err := s.AbandonOpenUsages(sess.ID); err != nil {
		t.Fatal(err)
	}
	stats, _ := s.SkillStats(sk.ID)
	if stats.TotalUses != 1 { // abandon conta como uso resolvido
		t.Fatalf("abandon não fechou uso: %+v", stats)
	}
}

func TestConsecutiveFailures(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sk, _ := s.CreateSkill(p.ID, "Deploy", "# v1")

	for i := 0; i < 3; i++ {
		uid, _ := s.RecordSkillUsageStart(sk.ID, nil, 1)
		_ = s.CloseSkillUsage(uid, "error", 1, false, 50)
	}
	n, err := s.ConsecutiveFailures(sk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("consecutive = %d, want 3", n)
	}

	// um sucesso reinicia a contagem
	uid, _ := s.RecordSkillUsageStart(sk.ID, nil, 1)
	_ = s.CloseSkillUsage(uid, "success", 0, false, 60)
	n, _ = s.ConsecutiveFailures(sk.ID)
	if n != 0 {
		t.Fatalf("consecutive após sucesso = %d", n)
	}
}
