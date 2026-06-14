package httpapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/apply"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/distill"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/mirror"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/retro"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
	"github.com/gorilla/websocket"
)

func TestRetroWSEvents(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	b := bus.New()
	eng := distill.New(s, &retroCLI{resp: "[]"}, b)
	applier := apply.New(s, mirror.New(t.TempDir()), b)
	obs := &retroFakeObs{sess: []adapter.ExternalSession{
		{Adapter: "claude-code", ExternalRef: "a", Dir: "/repo", Title: "t", UpdatedAt: time.Now().Add(-2 * 24 * time.Hour)},
	}}
	svc := retro.New(s, eng, applier, b, []retro.Observer{obs}, nil)
	srv := New(Deps{Store: s, Bus: b, Applier: applier, Retro: svc})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// prepara run com uma sessão pendente já atribuída a um projeto
	p, _ := s.CreateProject("App", "")
	run, _ := svc.Plan(retro.Scope{CLIs: []string{"claude-code"}, Dirs: []string{"/repo"}, WindowDays: 90})
	pend, _ := s.PendingRunSessions(run.ID)
	for _, sid := range pend {
		_ = s.SetRunSessionProject(run.ID, sid, p.ID)
	}

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/events"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal("ws dial:", err)
	}
	defer conn.Close()

	// dispara execução após a assinatura estar ativa
	go func() {
		time.Sleep(100 * time.Millisecond)
		_, _ = svc.Start(context.Background(), run.ID)
	}()

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	gotRetro := false
	for i := 0; i < 20 && !gotRetro; i++ {
		var ev map[string]any
		if err := conn.ReadJSON(&ev); err != nil {
			break
		}
		if typ, _ := ev["type"].(string); strings.HasPrefix(typ, "retro.") {
			gotRetro = true
		}
	}
	if !gotRetro {
		t.Fatal("nenhum evento retro.* recebido via websocket")
	}
}
