package mcpserver

import (
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/vault"
)

type fixedKey struct{ k []byte }

func (f fixedKey) MasterKey() ([]byte, error) { return f.k, nil }

func setupVault(t *testing.T) (*Service, *store.Store, *vault.Vault) {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	key := make([]byte, 32)
	v, _ := vault.New(fixedKey{k: key})
	svc := NewWithVault(s, bus.New(), v)
	return svc, s, v
}

func TestGetSecretAlwaysConcede(t *testing.T) {
	svc, s, v := setupVault(t)
	p, _ := s.CreateProject("App", "")
	ct, _ := v.Encrypt([]byte("VALOR-CRU"))
	sec, _ := s.CreateSecret(&store.Secret{ProjectID: p.ID, Name: "K", Mode: "value", Policy: "always"}, ct)
	res := svc.handleGetSecret("sess-1", p.ID, "K")
	if res.IsError {
		t.Fatalf("erro inesperado: %+v", res)
	}
	if got := textOf(res); got != "VALOR-CRU" {
		t.Fatalf("valor = %q", got)
	}
	rows, _ := s.ListSecretAudit(p.ID)
	if !hasAction(rows, "requested") || !hasAction(rows, "granted") {
		t.Fatalf("auditoria incompleta: %+v", rows)
	}
	_ = sec
}

func TestGetSecretRecipeSemAprovacao(t *testing.T) {
	svc, s, _ := setupVault(t)
	p, _ := s.CreateProject("App", "")
	s.CreateSecret(&store.Secret{ProjectID: p.ID, Name: "DB", Mode: "recipe",
		Recipe: "rode `op read op://db/url`", Policy: "per_access"}, nil)
	res := svc.handleGetSecret("sess-1", p.ID, "DB")
	if res.IsError || !strings.Contains(textOf(res), "op read") {
		t.Fatalf("receita não devolvida: %+v", res)
	}
}

func TestGetSecretPerAccessAprovado(t *testing.T) {
	svc, s, v := setupVault(t)
	p, _ := s.CreateProject("App", "")
	ct, _ := v.Encrypt([]byte("XYZ"))
	s.CreateSecret(&store.Secret{ProjectID: p.ID, Name: "K", Mode: "value", Policy: "per_access"}, ct)
	// aprova de forma assíncrona escutando o bus
	go func() {
		ch, cancel := svc.bus.Subscribe()
		defer cancel()
		ev := <-ch
		reqID := ev.Payload.(map[string]any)["request_id"].(string)
		time.Sleep(5 * time.Millisecond)
		v.Broker().Resolve(reqID, true)
	}()
	res := svc.handleGetSecret("sess-1", p.ID, "K")
	if res.IsError || textOf(res) != "XYZ" {
		t.Fatalf("esperava XYZ aprovado: %+v", res)
	}
}

func TestGetSecretPerAccessNegado(t *testing.T) {
	svc, s, v := setupVault(t)
	p, _ := s.CreateProject("App", "")
	ct, _ := v.Encrypt([]byte("XYZ"))
	s.CreateSecret(&store.Secret{ProjectID: p.ID, Name: "K", Mode: "value", Policy: "per_access"}, ct)
	go func() {
		ch, cancel := svc.bus.Subscribe()
		defer cancel()
		ev := <-ch
		v.Broker().Resolve(ev.Payload.(map[string]any)["request_id"].(string), false)
	}()
	res := svc.handleGetSecret("sess-1", p.ID, "K")
	if !res.IsError {
		t.Fatal("esperava erro em acesso negado")
	}
	rows, _ := s.ListSecretAudit(p.ID)
	if !hasAction(rows, "denied") {
		t.Fatalf("faltou auditoria denied: %+v", rows)
	}
}

func TestGetSecretPerSessionReusaGrant(t *testing.T) {
	svc, s, v := setupVault(t)
	p, _ := s.CreateProject("App", "")
	ct, _ := v.Encrypt([]byte("XYZ"))
	sec, _ := s.CreateSecret(&store.Secret{ProjectID: p.ID, Name: "K", Mode: "value", Policy: "per_session"}, ct)
	// simula grant prévio na sessão
	sid := "sess-1"
	s.AuditSecret(sec.ID, "K", &sid, &p.ID, "granted", "")
	// não deve perguntar (sem assinante para responder); concede direto
	res := svc.handleGetSecret(sid, p.ID, "K")
	if res.IsError || textOf(res) != "XYZ" {
		t.Fatalf("per_session com grant deveria conceder direto: %+v", res)
	}
}

func textOf(r *mcp.CallToolResult) string {
	if len(r.Content) == 0 {
		return ""
	}
	if tc, ok := r.Content[0].(*mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}

func hasAction(rows []*store.SecretAudit, action string) bool {
	for _, r := range rows {
		if r.Action == action {
			return true
		}
	}
	return false
}

func TestGetSecretPerAccessExpira(t *testing.T) {
	svc, s, v := setupVault(t)
	p, _ := s.CreateProject("App", "")
	ct, _ := v.Encrypt([]byte("XYZ"))
	s.CreateSecret(&store.Secret{ProjectID: p.ID, Name: "K", Mode: "value", Policy: "per_access"}, ct)
	// timeout mínimo configurável (1s) e NENHUM assinante respondendo → expira
	if err := s.SetSetting("secret_approval_timeout_s", "1"); err != nil {
		t.Fatal(err)
	}
	res := svc.handleGetSecret("sess-1", p.ID, "K")
	if !res.IsError {
		t.Fatal("esperava erro após expirar a aprovação")
	}
	rows, _ := s.ListSecretAudit(p.ID)
	if !hasAction(rows, "expired") {
		t.Fatalf("faltou auditoria expired: %+v", rows)
	}
}
