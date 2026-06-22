package httpapi

import (
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/agui"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func TestAttachInterpretation_LogsAudit(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	srv := &Server{deps: Deps{
		Bus:        bus.New(),
		Store:      st,
		Summarizer: &fakeHeadless{out: `{"kind":"text","prompt":"e agora?","options":[]}`},
	}, interpret: newInterpretCache()}

	snap := &agui.Snapshot{
		SessionID: "s1",
		State:     agui.StateAwaiting,
		Message:   "Quer que eu continue?",
		History:   []agui.HistoryLine{{Role: "assistant", Text: "Quer que eu continue?"}},
	}
	srv.attachInterpretation(snap)

	var got []*store.EngineLogEntry
	for i := 0; i < 100; i++ {
		got, _ = st.ListEngineLog(10)
		if len(got) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(got) != 1 || got[0].EngineID != "interpret" ||
		got[0].Input == "" || got[0].Output == "" {
		t.Fatalf("auditoria da interpretação incompleta: %+v", got)
	}
}
