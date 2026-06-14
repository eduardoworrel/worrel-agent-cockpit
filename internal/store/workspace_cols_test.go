package store

import "testing"

func TestWorkspaceDirAndClassify(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	if p.WorkspaceDir == "" {
		t.Fatal("CreateProject deveria preencher WorkspaceDir")
	}
	// sessão não-classificada com scratch
	sess, _ := s.CreateSession(&Session{Adapter: "claude-code", Mode: "wrapper", WorkspaceDir: "/tmp/scratch"})
	if sess.ProjectID != "" {
		t.Fatalf("nasceu classificada: %q", sess.ProjectID)
	}
	// classificar
	if err := s.ClassifySession(sess.ID, p.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetSession(sess.ID)
	if got.ProjectID != p.ID {
		t.Fatalf("classify falhou: %q", got.ProjectID)
	}
	// promover sessão a projeto
	sess2, _ := s.CreateSession(&Session{Adapter: "opencode", Mode: "wrapper"})
	np, err := s.PromoteSessionToProject(sess2.ID, "Novo Escopo", "desc")
	if err != nil {
		t.Fatal(err)
	}
	got2, _ := s.GetSession(sess2.ID)
	if got2.ProjectID != np.ID || np.Name != "Novo Escopo" {
		t.Fatalf("promote falhou: sess.proj=%q np=%+v", got2.ProjectID, np)
	}
}

func TestListActiveWrapperSessions(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.CreateSession(&Session{Adapter: "claude-code", Mode: "wrapper"})
	s.CreateSession(&Session{Adapter: "opencode", Mode: "observed"}) // não conta
	b, _ := s.CreateSession(&Session{Adapter: "opencode", Mode: "wrapper"})
	s.EndSession(b.ID) // não conta (ended)
	act, _ := s.ListActiveWrapperSessions()
	if len(act) != 1 || act[0].ID != a.ID {
		t.Fatalf("ativas = %+v", act)
	}
}
