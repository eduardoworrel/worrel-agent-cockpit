package store

import "testing"

func (s *Store) ListRetroClustersMust(t *testing.T, runID string) []*RetroCluster {
	l, err := s.ListRetroClusters(runID)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestRetroClusterAndSuppression(t *testing.T) {
	s := newTestStore(t)
	run, _ := s.CreateRetroRun(&RetroRun{})
	c, err := s.CreateRetroCluster(&RetroCluster{
		RunID: run.ID, Name: "API Pagamentos", Dirs: `["/a","/b"]`, SessionIDs: `["s1","s2"]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListRetroClusters(run.ID)
	if len(list) != 1 || list[0].Name != "API Pagamentos" {
		t.Fatalf("clusters %+v", list)
	}
	p, _ := s.CreateProject("Proj", "")
	if err := s.SetClusterDecision(c.ID, "approved", p.ID); err != nil {
		t.Fatal(err)
	}
	got := s.ListRetroClustersMust(t, run.ID)[0]
	if got.Decision != "approved" || got.ApprovedProjectID == nil || *got.ApprovedProjectID != p.ID {
		t.Fatalf("decisão %+v", got)
	}
	// supressão por hash
	if s.IsSecretSuppressed("abc") {
		t.Fatal("hash não deveria estar suprimido ainda")
	}
	if err := s.SuppressSecret("abc"); err != nil {
		t.Fatal(err)
	}
	if !s.IsSecretSuppressed("abc") {
		t.Fatal("hash deveria estar suprimido")
	}
	if err := s.SuppressSecret("abc"); err != nil {
		t.Fatal(err) // idempotente
	}
}
