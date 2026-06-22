package httpapi

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/agui"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// fakeHeadless devolve uma saída canned e conta as chamadas.
type fakeHeadless struct {
	out   string
	calls int32
}

func (f *fakeHeadless) RunHeadless(_ context.Context, _ string, _ adapter.HeadlessOpts) (string, error) {
	atomic.AddInt32(&f.calls, 1)
	return f.out, nil
}

func newProgressServer(sum HeadlessLLM) *Server {
	return &Server{deps: Deps{Bus: bus.New(), Summarizer: sum}, progress: newProgressCache()}
}

func waitProgress(t *testing.T, s *Server, id string) []string {
	t.Helper()
	for i := 0; i < 100; i++ {
		if lines, _ := s.progress.get(id); len(lines) > 0 {
			return lines
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timeout esperando o resumo assíncrono")
	return nil
}

func evp(role, kind, content string) *store.TranscriptEvent {
	return &store.TranscriptEvent{Role: role, Kind: kind, Content: content}
}

func TestAttachProgress_GeneratesAndCaches(t *testing.T) {
	fake := &fakeHeadless{out: "agente está lendo o db\nencontrou a senha e fará o update"}
	s := newProgressServer(fake)
	events := []*store.TranscriptEvent{evp("user", "text", "atualize o db"), evp("assistant", "tool_use", "Bash x")}

	snap := agui.Snapshot{SessionID: "s1", State: agui.StateAwaiting}
	s.attachProgress(&snap, events) // dispara goroutine

	lines := waitProgress(t, s, "s1")
	if len(lines) != 2 || lines[0] != "agente está lendo o db" {
		t.Fatalf("linhas = %#v", lines)
	}

	// segundo snapshot (mesmo tamanho de transcript) reusa o cache, sem novo LLM.
	snap2 := agui.Snapshot{SessionID: "s1", State: agui.StateAwaiting}
	s.attachProgress(&snap2, events)
	if len(snap2.Progress) != 2 {
		t.Fatalf("snapshot deve trazer o cache: %#v", snap2.Progress)
	}
	time.Sleep(20 * time.Millisecond)
	if c := atomic.LoadInt32(&fake.calls); c != 1 {
		t.Fatalf("esperava 1 chamada ao LLM (cache), veio %d", c)
	}
}

func TestAttachProgress_SkipsWhenEnded(t *testing.T) {
	fake := &fakeHeadless{out: "x\ny"}
	s := newProgressServer(fake)
	snap := agui.Snapshot{SessionID: "s1", State: agui.StateEnded}
	s.attachProgress(&snap, []*store.TranscriptEvent{evp("user", "text", "oi"), evp("assistant", "text", "ok")})
	time.Sleep(20 * time.Millisecond)
	if c := atomic.LoadInt32(&fake.calls); c != 0 {
		t.Fatalf("sessão encerrada não deve resumir, veio %d chamadas", c)
	}
}

func TestAttachProgress_LogsAudit(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	// Plano 3: o resumo agora é gated por EngineEnabled (default OFF); liga aqui.
	_ = st.SetEngineConfig("summary", "__enabled", "true", "")
	srv := &Server{deps: Deps{
		Bus:        bus.New(),
		Store:      st,
		Summarizer: &fakeHeadless{out: `{"title":"Foo","lines":["fez X"]}`},
	}, progress: newProgressCache()}

	snap := &agui.Snapshot{SessionID: "s1", State: agui.StateAwaiting}
	events := []*store.TranscriptEvent{
		{Role: "user", Kind: "text", Content: "oi"},
		{Role: "assistant", Kind: "text", Content: "fazendo X"},
		{Role: "assistant", Kind: "text", Content: "fazendo Y"},
	}
	srv.attachProgress(snap, events)

	// a geração é assíncrona; espera curta determinística por polling do log.
	var got []*store.EngineLogEntry
	for i := 0; i < 100; i++ {
		got, _ = st.ListEngineLog(10)
		if len(got) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(got) != 1 {
		t.Fatalf("esperava 1 linha de auditoria, veio %d", len(got))
	}
	if got[0].EngineID != "summary" || got[0].SessionID != "s1" ||
		got[0].Input == "" || got[0].Output == "" {
		t.Fatalf("auditoria incompleta: %+v", got[0])
	}
}

func TestAttachProgress_DisabledSkipsLLM(t *testing.T) {
	st, _ := store.Open(t.TempDir() + "/t.db")
	defer st.Close()
	// summary OFF global (default já é false, mas fixamos explícito):
	_ = st.SetEngineConfig("summary", "__enabled", "false", "")

	sum := &fakeHeadless{out: `{"title":"X","lines":["y"]}`}
	srv := &Server{deps: Deps{Bus: bus.New(), Store: st, Summarizer: sum}, progress: newProgressCache()}

	snap := &agui.Snapshot{SessionID: "s1", State: agui.StateAwaiting}
	events := []*store.TranscriptEvent{
		{Role: "user", Kind: "text", Content: "oi"},
		{Role: "assistant", Kind: "text", Content: "a"},
		{Role: "assistant", Kind: "text", Content: "b"},
	}
	srv.attachProgress(snap, events)

	// dá tempo de uma eventual goroutine indevida rodar
	time.Sleep(50 * time.Millisecond)
	got, _ := st.ListEngineLog(10)
	if len(got) != 0 {
		t.Fatalf("summary OFF não deveria gerar auditoria/chamada; veio %d", len(got))
	}
	if c := atomic.LoadInt32(&sum.calls); c != 0 {
		t.Fatalf("summary OFF não deveria chamar o LLM; veio %d", c)
	}
}

func TestAttachProgress_NoSummarizerNoop(t *testing.T) {
	s := newProgressServer(nil)
	snap := agui.Snapshot{SessionID: "s1", State: agui.StateAwaiting}
	s.attachProgress(&snap, []*store.TranscriptEvent{evp("user", "text", "a"), evp("assistant", "text", "b")})
	if snap.Progress != nil {
		t.Fatalf("sem summarizer, progress deve ficar nil: %#v", snap.Progress)
	}
}

// countingAdapter é um adapter fake headless (ID "claude-code") usado para
// confirmar que summarizerFor escolhe o adapter configurado em vez do fallback.
type countingAdapter struct {
	baseFakeAdapter
}

func (countingAdapter) ID() string { return "claude-code" }
func (countingAdapter) Capabilities() adapter.Caps {
	return adapter.Caps{Headless: true}
}

func TestSummarizerFor_UsesConfiguredHarness(t *testing.T) {
	st, _ := store.Open(t.TempDir() + "/t.db")
	defer st.Close()
	_ = st.SetEngineConfig("summary", "harness", "claude-code", "")
	_ = st.SetEngineConfig("summary", "model", "claude-sonnet-4-6", "")

	reg := adapter.NewRegistry()
	chosen := &countingAdapter{}
	reg.Register(chosen)

	srv := &Server{deps: Deps{
		Store: st, Adapters: reg,
		Summarizer: &fakeHeadless{out: "fallback"},
	}}
	llm, opts := srv.summarizerFor("summary", "")
	if llm != HeadlessLLM(chosen) {
		t.Fatal("deveria escolher o adapter configurado (claude-code)")
	}
	if opts.Model != "claude-sonnet-4-6" {
		t.Fatalf("model não propagado: %q", opts.Model)
	}
}

func TestSummarizerFor_FallsBackToSummarizer(t *testing.T) {
	st, _ := store.Open(t.TempDir() + "/t.db")
	defer st.Close()
	sum := &fakeHeadless{out: "fallback"}
	srv := &Server{deps: Deps{Store: st, Adapters: adapter.NewRegistry(), Summarizer: sum}}
	llm, opts := srv.summarizerFor("summary", "")
	if llm != HeadlessLLM(sum) {
		t.Fatal("sem harness configurado, deve cair no Summarizer")
	}
	if opts.Model != "" {
		t.Fatalf("sem model configurado, opts.Model deveria ser vazio: %q", opts.Model)
	}
}
