package scheduler

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/engine"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

type fakeEngine struct{ ran [][2]string }

func (f *fakeEngine) Spec() engine.Spec {
	return engine.Spec{ID: "fake", Name: "Fake", Triggers: []engine.Trigger{engine.TriggerPeriodic}, OutputType: "suggestion", DefaultOn: false}
}
func (f *fakeEngine) Run(_ context.Context, rc engine.RunContext) error {
	f.ran = append(f.ran, [2]string{rc.ProjectID, rc.SessionID})
	return nil
}

func TestSchedulerRunsEnabledOnEndedSessions(t *testing.T) {
	st, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	p, _ := st.CreateProject("App", "")
	ended, _ := st.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper", Status: "ended"})

	fe := &fakeEngine{}
	reg := engine.NewRegistry()
	reg.Register(fe)
	sch := New(reg, st)

	// desabilitado por default → não roda
	sch.Tick(context.Background())
	if len(fe.ran) != 0 {
		t.Fatalf("desabilitado não deveria rodar: %+v", fe.ran)
	}

	// habilita globalmente → roda na sessão encerrada, uma vez
	_ = st.SetEngineConfig("fake", "__enabled", "true", "")
	sch.Tick(context.Background())
	if len(fe.ran) != 1 || fe.ran[0] != [2]string{p.ID, ended.ID} {
		t.Fatalf("deveria rodar 1x na encerrada: %+v", fe.ran)
	}
	// segundo tick: já processada, não re-roda
	sch.Tick(context.Background())
	if len(fe.ran) != 1 {
		t.Fatalf("não deveria re-rodar: %+v", fe.ran)
	}
}
