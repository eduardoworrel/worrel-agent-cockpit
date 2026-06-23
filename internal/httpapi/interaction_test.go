package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/agui"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/apply"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/ask"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/mirror"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/wrapper"
)

// TestInteractionSnapshot cobre a borda HTTP do canal AG-UI: cria uma sessão com
// transcript, lê o snapshot e confere o contexto traduzido.
func TestInteractionSnapshot(t *testing.T) {
	ts, s, _ := newSessionsServer(t)
	sess, err := s.CreateSession(&store.Session{Mode: "wrapper"})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.AppendTranscriptEventRich(sess.ID, "user", "text", "rode os testes", "", 0, 0)
	_ = s.AppendTranscriptEventRich(sess.ID, "assistant", "tool_use", "Bash {\"command\":\"go test\"}", "", 0, 0)
	_ = s.AppendTranscriptEventRich(sess.ID, "assistant", "text", "testes passaram, sigo?", "", 0, 0)

	resp, err := ts.Client().Get(ts.URL + "/api/sessions/" + sess.ID + "/interaction")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("GET interaction: err=%v status=%d", err, resp.StatusCode)
	}
	var snap agui.Snapshot
	_ = json.NewDecoder(resp.Body).Decode(&snap)

	if snap.Message != "testes passaram, sigo?" {
		t.Fatalf("message = %q", snap.Message)
	}
	if snap.UserMessage != "rode os testes" {
		t.Fatalf("user_message = %q", snap.UserMessage)
	}
	if len(snap.ToolCalls) != 1 || snap.ToolCalls[0].Name != "Bash" {
		t.Fatalf("tool_calls = %+v", snap.ToolCalls)
	}
	// PTY não está rodando neste teste → sessão considerada encerrada.
	if snap.State != agui.StateEnded {
		t.Fatalf("state = %q, want ended (sem PTY vivo)", snap.State)
	}
}

// TestInteractionAskSurfacesAndResolves cobre a regressão do modal de resposta:
// um ask pendente no broker (hook PreToolUse / MCP ask_user) deve (1) aparecer
// como interrupt no snapshot da sessão e (2) ser resolvido pelo respond via
// request_id. Guarda o caminho "broker primeiro" do handleInteractionRespond.
func TestInteractionAskSurfacesAndResolves(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	s.SetDataDir(dir)
	t.Cleanup(func() { s.Close() })
	m := mirror.New(t.TempDir())
	bs := bus.New()
	wm := wrapper.New(s, bs)
	b := ask.New()
	srv := New(Deps{Store: s, Mirror: m, Bus: bs, Applier: apply.New(s, m, bs), Wrapper: wm, Ask: b})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	sess, _ := s.CreateSession(&store.Session{Adapter: "claude-code", Mode: "wrapper"})
	req, _ := b.Open(ask.Request{SessionID: sess.ID, Kind: "permission", Title: "Rodar comando", Detail: "echo oi"})

	// (1) snapshot expõe o ask como interrupt.
	resp := doJSON(t, ts, "GET", "/api/sessions/"+sess.ID+"/interaction", nil)
	var snap agui.Snapshot
	json.NewDecoder(resp.Body).Decode(&snap)
	if snap.Interrupt == nil || snap.Interrupt.RequestID != req.ID {
		t.Fatalf("interrupt = %+v, want request_id %s", snap.Interrupt, req.ID)
	}
	if snap.Interrupt.Kind != agui.KindPermission {
		t.Fatalf("kind = %q, want permission", snap.Interrupt.Kind)
	}

	// (2) respond resolve o ask pelo broker → 204 e some de pending.
	resp = doJSON(t, ts, "POST", "/api/sessions/"+sess.ID+"/interaction/respond",
		map[string]any{"request_id": req.ID, "answer": "allow"})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("respond = %d, want 204", resp.StatusCode)
	}
	if len(b.Pending()) != 0 {
		t.Fatalf("pending após respond = %d, want 0", len(b.Pending()))
	}
}

// TestInteractionPromptNotRunning: injetar prompt numa sessão sem PTY → 409.
func TestInteractionPromptNotRunning(t *testing.T) {
	ts, s, _ := newSessionsServer(t)
	sess, _ := s.CreateSession(&store.Session{Mode: "wrapper"})
	resp, err := ts.Client().Post(
		ts.URL+"/api/sessions/"+sess.ID+"/interaction/prompt",
		"application/json", strings.NewReader(`{"text":"oi"}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
}

// TestInteractionPromptEmpty: corpo sem texto → 400.
func TestInteractionPromptEmpty(t *testing.T) {
	ts, s, _ := newSessionsServer(t)
	sess, _ := s.CreateSession(&store.Session{Mode: "wrapper"})
	resp, _ := ts.Client().Post(
		ts.URL+"/api/sessions/"+sess.ID+"/interaction/prompt",
		"application/json", strings.NewReader(`{"text":"  "}`))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}
