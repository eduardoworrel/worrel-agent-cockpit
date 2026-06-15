package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/apply"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/ask"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/mirror"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func newAskTestServer(t *testing.T) (*httptest.Server, *store.Store, *ask.Broker) {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	m := mirror.New(t.TempDir())
	b := ask.New()
	srv := New(Deps{Store: s, Mirror: m, Bus: bus.New(), Applier: apply.New(s, m, bus.New()), Ask: b})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, s, b
}

func TestAskRespondUnknownIs404(t *testing.T) {
	ts, _, _ := newAskTestServer(t)
	resp := doJSON(t, ts, "POST", "/api/asks/nope/respond", map[string]any{"answer": "allow"})
	if resp.StatusCode != 404 {
		t.Fatalf("respond unknown = %d", resp.StatusCode)
	}
}

func TestAskPendingReflectsBroker(t *testing.T) {
	ts, _, b := newAskTestServer(t)
	b.Open(ask.Request{SessionID: "s1", Kind: "choice", Title: "A ou B?"})
	resp := doJSON(t, ts, "GET", "/api/asks/pending", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("pending = %d", resp.StatusCode)
	}
	var got []ask.Request
	json.NewDecoder(resp.Body).Decode(&got)
	if len(got) != 1 || got[0].Title != "A ou B?" {
		t.Fatalf("pending body = %+v", got)
	}
}

func TestPermissionRequestBlocksThenDecides(t *testing.T) {
	ts, s, b := newAskTestServer(t)
	sess, _ := s.CreateSession(&store.Session{Adapter: "claude-code", Mode: "wrapper"})

	type decisionResp struct {
		Decision string `json:"decision"`
	}
	done := make(chan decisionResp, 1)
	go func() {
		resp := doJSON(t, ts, "POST", "/api/sessions/"+sess.ID+"/permission-request",
			map[string]any{"tool": "Bash", "input": map[string]any{"command": "npm test"}})
		var dr decisionResp
		json.NewDecoder(resp.Body).Decode(&dr)
		done <- dr
	}()

	var reqID string
	for i := 0; i < 100; i++ {
		if p := b.Pending(); len(p) == 1 {
			reqID = p[0].ID
			if p[0].Kind != "permission" || p[0].Title != "Rodar comando" || p[0].Detail != "npm test" {
				t.Fatalf("pending = %+v", p[0])
			}
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if reqID == "" {
		t.Fatal("permission request never reached the broker")
	}
	doJSON(t, ts, "POST", "/api/asks/"+reqID+"/respond", map[string]any{"answer": "allow"})

	select {
	case dr := <-done:
		if dr.Decision != "allow" {
			t.Fatalf("decision = %q", dr.Decision)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("permission-request did not return after respond")
	}
}
