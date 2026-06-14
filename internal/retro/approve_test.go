package retro

import (
	"encoding/json"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func mkCluster(t *testing.T, s *store.Store, runID string, sessIDs []string, existing string) *store.RetroCluster {
	t.Helper()
	sj, _ := json.Marshal(sessIDs)
	c := &store.RetroCluster{RunID: runID, Name: "C", SessionIDs: string(sj)}
	if existing != "" {
		c.ExistingProjectID = &existing
	}
	got, err := s.CreateRetroCluster(c)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func runSessProject(t *testing.T, s *store.Store, runID, sessID string) string {
	t.Helper()
	var pid string
	_ = s.DB().QueryRow(`SELECT COALESCE(project_id,'') FROM retro_run_sessions WHERE run_id=? AND session_id=?`, runID, sessID).Scan(&pid)
	return pid
}

func TestApproveCreateAndMergeAndAssociate(t *testing.T) {
	s := newStore(t)
	run, _ := s.CreateRetroRun(&store.RetroRun{Status: "clustered"})
	mk := func() string {
		se, _ := s.CreateSession(&store.Session{Adapter: "claude-code", Mode: "observed"})
		_ = s.EndSession(se.ID)
		_ = s.AddRunSession(run.ID, se.ID, "")
		return se.ID
	}
	s1, s2, s3 := mk(), mk(), mk()
	ap := NewApprover(s)

	// ApproveCluster cria projeto e propaga project_id
	cA := mkCluster(t, s, run.ID, []string{s1}, "")
	pidA, err := ap.ApproveCluster(cA.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if runSessProject(t, s, run.ID, s1) != pidA {
		t.Fatal("project_id não propagado em approve")
	}

	// MergeClusters: dois clusters → um projeto com todas as sessões (critério 5)
	cB := mkCluster(t, s, run.ID, []string{s2}, "")
	cC := mkCluster(t, s, run.ID, []string{s3}, "")
	pidM, err := ap.MergeClusters(run.ID, []string{cB.ID, cC.ID}, "Fundido", "")
	if err != nil {
		t.Fatal(err)
	}
	if runSessProject(t, s, run.ID, s2) != pidM || runSessProject(t, s, run.ID, s3) != pidM {
		t.Fatal("merge não propagou para ambas sessões")
	}

	// Associar a projeto existente não cria projeto novo (critério 6)
	existing, _ := s.CreateProject("Existente", "")
	before, _ := s.ListProjects()
	se4, _ := s.CreateSession(&store.Session{Adapter: "claude-code", Mode: "observed"})
	_ = s.EndSession(se4.ID)
	_ = s.AddRunSession(run.ID, se4.ID, "")
	cD := mkCluster(t, s, run.ID, []string{se4.ID}, existing.ID)
	pidD, err := ap.ApproveCluster(cD.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if pidD != existing.ID {
		t.Fatalf("não associou ao existente: %s", pidD)
	}
	after, _ := s.ListProjects()
	if len(after) != len(before) {
		t.Fatalf("criou projeto novo ao associar: %d -> %d", len(before), len(after))
	}

	// Discard mantém sessões sem project_id
	se5, _ := s.CreateSession(&store.Session{Adapter: "claude-code", Mode: "observed"})
	_ = s.EndSession(se5.ID)
	_ = s.AddRunSession(run.ID, se5.ID, "")
	cE := mkCluster(t, s, run.ID, []string{se5.ID}, "")
	if err := ap.DiscardCluster(cE.ID); err != nil {
		t.Fatal(err)
	}
	if runSessProject(t, s, run.ID, se5.ID) != "" {
		t.Fatal("discard não deveria atribuir project_id")
	}
}
