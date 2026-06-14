package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/apply"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/mirror"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/vault"
)

type fixedKeyHTTP struct{ k []byte }

func (f fixedKeyHTTP) MasterKey() ([]byte, error) { return f.k, nil }

func newSecretTestServer(t *testing.T) (*httptest.Server, *store.Store, *vault.Vault) {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	key := make([]byte, 32)
	v, _ := vault.New(fixedKeyHTTP{k: key})
	m := mirror.New(t.TempDir())
	srv := New(Deps{Store: s, Mirror: m, Bus: bus.New(), Applier: apply.New(s, m, bus.New()), Vault: v})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, s, v
}

func doJSON(t *testing.T, ts *httptest.Server, method, path string, body any) *http.Response {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, ts.URL+path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestCreateAndListSecret(t *testing.T) {
	ts, s, _ := newSecretTestServer(t)
	p, _ := s.CreateProject("App", "")
	resp := doJSON(t, ts, "POST", "/api/projects/"+p.ID+"/secrets", map[string]any{
		"name": "API_KEY", "mode": "value", "value": "segredo123", "policy": "always",
	})
	if resp.StatusCode != 201 {
		t.Fatalf("create = %d", resp.StatusCode)
	}
	var sec store.Secret
	json.NewDecoder(resp.Body).Decode(&sec)
	if sec.ID == "" || sec.Name != "API_KEY" {
		t.Fatalf("secret = %+v", sec)
	}
	// valor NÃO deve aparecer na resposta
	body, _ := json.Marshal(sec)
	if strings.Contains(string(body), "segredo123") {
		t.Fatal("valor em claro na resposta REST!")
	}

	resp2 := doJSON(t, ts, "GET", "/api/projects/"+p.ID+"/secrets", nil)
	if resp2.StatusCode != 200 {
		t.Fatalf("list = %d", resp2.StatusCode)
	}
	var list []store.Secret
	json.NewDecoder(resp2.Body).Decode(&list)
	if len(list) != 1 {
		t.Fatalf("list len = %d", len(list))
	}
}

func TestDeleteSecret(t *testing.T) {
	ts, s, _ := newSecretTestServer(t)
	p, _ := s.CreateProject("App", "")
	sec, _ := s.CreateSecret(&store.Secret{ProjectID: p.ID, Name: "X", Mode: "value"}, nil)
	resp := doJSON(t, ts, "DELETE", "/api/secrets/"+sec.ID, nil)
	if resp.StatusCode != 204 {
		t.Fatalf("delete = %d", resp.StatusCode)
	}
}

func TestApproveSecretEndpoint(t *testing.T) {
	ts, _, v := newSecretTestServer(t)
	// Abre um pedido no broker diretamente
	reqID, ch := v.Broker().Open()
	resp := doJSON(t, ts, "POST", "/api/secret-approvals/"+reqID, map[string]any{
		"approve": true,
	})
	if resp.StatusCode != 200 {
		t.Fatalf("approve = %d", resp.StatusCode)
	}
	if ok := <-ch; !ok {
		t.Fatal("broker não recebeu aprovação")
	}
}

func TestSecretApprovalEndpoint(t *testing.T) {
	ts, _, _ := newSecretTestServer(t)
	// id inexistente devolve 404/erro mas sem panic
	resp := doJSON(t, ts, "POST", "/api/secret-approvals/nao-existe", map[string]any{
		"approve": true,
	})
	if resp.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestSetInjection(t *testing.T) {
	ts, s, _ := newSecretTestServer(t)
	p, _ := s.CreateProject("App", "")
	resp := doJSON(t, ts, "PUT", "/api/projects/"+p.ID+"/secrets/injection", map[string]any{
		"enabled": true,
	})
	if resp.StatusCode != 204 {
		t.Fatalf("injection = %d", resp.StatusCode)
	}
	if !s.InjectionEnabled(p.ID) {
		t.Fatal("injeção não habilitada após PUT")
	}
}

func TestRecipeVaiParaMemoria(t *testing.T) {
	ts, s, _ := newSecretTestServer(t)
	p, _ := s.CreateProject("App", "")
	resp := doJSON(t, ts, "POST", "/api/projects/"+p.ID+"/secrets", map[string]any{
		"name":   "DB_URL",
		"mode":   "recipe",
		"recipe": "rode `op read op://db/url`",
	})
	if resp.StatusCode != 201 {
		t.Fatalf("create recipe = %d", resp.StatusCode)
	}
	// aguarda a goroutine de espelhamento (best-effort, até 200ms)
	for i := 0; i < 20; i++ {
		mem, _ := s.GetMemory(p.ID)
		if mem != nil && strings.Contains(mem.Content, "op read") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	mem, _ := s.GetMemory(p.ID)
	if mem == nil || !strings.Contains(mem.Content, "op read") {
		var content string
		if mem != nil {
			content = mem.Content
		}
		t.Fatalf("receita não espelhada na memória: %q", content)
	}
}

func TestListSecretAudit(t *testing.T) {
	ts, s, _ := newSecretTestServer(t)
	p, _ := s.CreateProject("App", "")
	sec, _ := s.CreateSecret(&store.Secret{ProjectID: p.ID, Name: "K", Mode: "value"}, nil)
	sid := "sess-1"
	s.AuditSecret(sec.ID, "K", &sid, &p.ID, "granted", "")
	resp := doJSON(t, ts, "GET", "/api/projects/"+p.ID+"/secrets/audit", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("audit = %d", resp.StatusCode)
	}
	var rows []store.SecretAudit
	json.NewDecoder(resp.Body).Decode(&rows)
	if len(rows) != 1 || rows[0].Action != "granted" {
		t.Fatalf("audit rows = %+v", rows)
	}
}
