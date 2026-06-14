package store

import (
	"database/sql"
	"testing"
)

func TestCreateAndGetSecret(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sec, err := s.CreateSecret(&Secret{
		ProjectID: p.ID, Name: "API_KEY", Mode: "value", Policy: "always",
	}, []byte("ciphertext-fake"))
	if err != nil {
		t.Fatal(err)
	}
	if sec.ID == "" {
		t.Fatal("id vazio")
	}
	got, err := s.GetSecret(sec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "API_KEY" || !got.HasValue {
		t.Fatalf("inesperado: %+v", got)
	}
}

func TestListSecrets(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	s.CreateSecret(&Secret{ProjectID: p.ID, Name: "B", Mode: "value"}, nil)
	s.CreateSecret(&Secret{ProjectID: p.ID, Name: "A", Mode: "value"}, nil)
	list, err := s.ListSecrets(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Name != "A" {
		t.Fatalf("lista = %+v", list)
	}
}

func TestResolveSecretFallbackGlobal(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	s.CreateSecret(&Secret{Name: "GLOBAL", Mode: "value"}, []byte("ct"))
	got, err := s.ResolveSecret(p.ID, "GLOBAL")
	if err != nil || got.ProjectID != "" {
		t.Fatalf("fallback global falhou: %v %+v", err, got)
	}
}

func TestResolveSecretProjetoSobrePoeGlobal(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	s.CreateSecret(&Secret{Name: "K", Mode: "value"}, []byte("global"))
	s.CreateSecret(&Secret{ProjectID: p.ID, Name: "K", Mode: "value"}, []byte("local"))
	got, err := s.ResolveSecret(p.ID, "K")
	if err != nil || got.ProjectID != p.ID {
		t.Fatalf("projeto não sobrepôs global: %v %+v", err, got)
	}
}

func TestDeleteSecret(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sec, _ := s.CreateSecret(&Secret{ProjectID: p.ID, Name: "X", Mode: "value"}, nil)
	if err := s.DeleteSecret(sec.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSecret(sec.ID); err != sql.ErrNoRows {
		t.Fatalf("esperava ErrNoRows, got %v", err)
	}
}

func TestAuditSecretPermanente(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sec, _ := s.CreateSecret(&Secret{ProjectID: p.ID, Name: "K", Mode: "value"}, nil)
	sid := "sess-1"
	s.AuditSecret(sec.ID, "K", &sid, &p.ID, "requested", "")
	s.AuditSecret(sec.ID, "K", &sid, &p.ID, "granted", "")
	s.DeleteSecret(sec.ID)
	rows, err := s.ListSecretAudit(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("auditoria deve sobreviver à remoção do segredo, got %d", len(rows))
	}
}

func TestHasSessionGrant(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sec, _ := s.CreateSecret(&Secret{ProjectID: p.ID, Name: "K", Mode: "value"}, nil)
	sid := "sess-abc"
	if s.HasSessionGrant(sid, sec.ID) {
		t.Fatal("não deveria ter grant ainda")
	}
	s.AuditSecret(sec.ID, "K", &sid, &p.ID, "granted", "")
	if !s.HasSessionGrant(sid, sec.ID) {
		t.Fatal("deveria ter grant após auditoria")
	}
}

func TestGlobalSecretUnicidade(t *testing.T) {
	s := newTestStore(t)
	s.CreateSecret(&Secret{Name: "GLOBAL", Mode: "value"}, nil)
	_, err := s.CreateSecret(&Secret{Name: "GLOBAL", Mode: "value"}, nil)
	if err == nil {
		t.Fatal("deveria rejeitar duplicata global")
	}
}
