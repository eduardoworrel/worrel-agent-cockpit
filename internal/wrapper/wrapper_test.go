package wrapper

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSpawnEchoAndStdin(t *testing.T) {
	st := newStore(t)
	b := bus.New()
	m := New(st, b)
	sess, _ := st.CreateSession(&store.Session{Adapter: "fake", Mode: "wrapper"})

	// /bin/cat ecoa stdin de volta no PTY
	if err := m.Spawn(sess.ID, adapter.CmdSpec{Path: "/bin/cat", Dir: "/tmp"}); err != nil {
		t.Fatal(err)
	}
	out := make(chan []byte, 64)
	unsub := m.Subscribe(sess.ID, func(p []byte) { out <- append([]byte(nil), p...) })
	defer unsub()

	if err := m.Write(sess.ID, []byte("ping\n")); err != nil {
		t.Fatal(err)
	}
	got := waitFor(t, out, "ping")
	if !strings.Contains(got, "ping") {
		t.Fatalf("eco = %q", got)
	}
	// output bruto acumulado contém o eco
	if !strings.Contains(string(m.RawOutput(sess.ID)), "ping") {
		t.Fatal("RawOutput sem eco")
	}
	if err := m.Kill(sess.ID); err != nil {
		t.Fatal(err)
	}
}

func TestExitMarksSessionEnded(t *testing.T) {
	st := newStore(t)
	b := bus.New()
	ch, cancel := b.Subscribe()
	defer cancel()
	m := New(st, b)
	sess, _ := st.CreateSession(&store.Session{Adapter: "fake", Mode: "wrapper"})

	// `sh -c exit 0` termina sozinho
	if err := m.Spawn(sess.ID, adapter.CmdSpec{Path: "/bin/sh", Args: []string{"-c", "exit 0"}, Dir: "/tmp"}); err != nil {
		t.Fatal(err)
	}
	// espera evento session.ended
	deadline := time.After(3 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.Type == "session.ended" {
				got, _ := st.GetSession(sess.ID)
				if got.Status != "ended" {
					t.Fatalf("status = %q", got.Status)
				}
				return
			}
		case <-deadline:
			t.Fatal("timeout esperando session.ended")
		}
	}
}

func TestResizeAndUnknownSession(t *testing.T) {
	m := New(newStore(t), bus.New())
	if err := m.Resize("nope", 80, 24); err == nil {
		t.Fatal("Resize de sessão inexistente deveria falhar")
	}
	if err := m.Write("nope", []byte("x")); err == nil {
		t.Fatal("Write de sessão inexistente deveria falhar")
	}
}

func TestRawOutputCappedAtTail(t *testing.T) {
	st := newStore(t)
	m := New(st, bus.New())
	sess, _ := st.CreateSession(&store.Session{Adapter: "fake", Mode: "wrapper"})

	// gera ~3MB de 'a' + marcador de cauda, depois dorme (a sessão segue viva
	// para podermos ler RawOutput antes do onExit remover a sessão do map).
	script := `yes a | head -c 3000000; printf WORREL-TAIL; sleep 30`
	if err := m.Spawn(sess.ID, adapter.CmdSpec{Path: "/bin/sh", Args: []string{"-c", script}, Dir: "/tmp"}); err != nil {
		t.Fatal(err)
	}
	defer m.Kill(sess.ID)

	deadline := time.Now().Add(10 * time.Second)
	var raw []byte
	for time.Now().Before(deadline) {
		raw = m.RawOutput(sess.ID)
		if strings.Contains(string(raw), "WORREL-TAIL") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(string(raw), "WORREL-TAIL") {
		t.Fatalf("timeout esperando marcador de cauda; len=%d", len(raw))
	}
	if len(raw) > maxRawBytes {
		t.Fatalf("RawOutput = %d bytes, deve ser <= %d", len(raw), maxRawBytes)
	}
	// é a CAUDA: com 3MB escritos e cap de 2MB, o buffer deve estar ~cheio
	if len(raw) < maxRawBytes/2 {
		t.Fatalf("RawOutput = %d bytes, esperava cauda próxima do cap", len(raw))
	}
}

func TestKillTerminatesProcessGroup(t *testing.T) {
	st := newStore(t)
	b := bus.New()
	ch, cancel := b.Subscribe()
	defer cancel()
	m := New(st, b)
	sess, _ := st.CreateSession(&store.Session{Adapter: "fake", Mode: "wrapper"})

	// sh spawna dois filhos sleep e espera — grupo com 3 processos.
	script := `sleep 30 & sleep 30 & wait`
	if err := m.Spawn(sess.ID, adapter.CmdSpec{Path: "/bin/sh", Args: []string{"-c", script}, Dir: "/tmp"}); err != nil {
		t.Fatal(err)
	}

	s, err := m.get(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	pgid := s.cmd.Process.Pid

	// espera os filhos aparecerem no grupo (sh + 2 sleeps)
	procDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(procDeadline) {
		out, _ := exec.Command("pgrep", "-g", fmt.Sprint(pgid)).Output()
		if len(strings.Fields(string(out))) >= 3 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if err := m.Kill(sess.ID); err != nil {
		t.Fatal(err)
	}

	// todo o grupo deve sumir (pgrep -g sem matches → exit != 0, output vazio)
	goneDeadline := time.Now().Add(5 * time.Second)
	for {
		out, err := exec.Command("pgrep", "-g", fmt.Sprint(pgid)).Output()
		if err != nil && len(strings.TrimSpace(string(out))) == 0 {
			break // nenhum processo restante no grupo
		}
		if time.Now().After(goneDeadline) {
			t.Fatalf("processos ainda vivos no grupo %d: %q", pgid, out)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// e a sessão é marcada como encerrada
	evDeadline := time.After(3 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.Type == "session.ended" {
				return
			}
		case <-evDeadline:
			t.Fatal("timeout esperando session.ended")
		}
	}
}

func waitFor(t *testing.T, ch chan []byte, sub string) string {
	t.Helper()
	var acc strings.Builder
	deadline := time.After(3 * time.Second)
	for {
		select {
		case p := <-ch:
			acc.Write(p)
			if strings.Contains(acc.String(), sub) {
				return acc.String()
			}
		case <-deadline:
			t.Fatalf("timeout esperando %q; got %q", sub, acc.String())
			return ""
		}
	}
}

// contextAdapter é um adapter.Adapter mínimo que retorna valores de contexto fixos.
type contextAdapter struct {
	used, limit int
}

func (c *contextAdapter) ID() string                                              { return "fake-ctx" }
func (c *contextAdapter) Detect() (adapter.Installed, error)                     { return adapter.Installed{}, nil }
func (c *contextAdapter) Capabilities() adapter.Caps                             { return adapter.Caps{} }
func (c *contextAdapter) BuildInteractive(opts adapter.SpawnOpts) (adapter.CmdSpec, error) {
	return adapter.CmdSpec{Path: "/bin/cat", Dir: "/tmp"}, nil
}
func (c *contextAdapter) RunHeadless(ctx context.Context, prompt string, opts adapter.HeadlessOpts) (string, error) {
	return "", nil
}
func (c *contextAdapter) DiscoverSessions(since time.Time) ([]adapter.ExternalSession, error) {
	return nil, adapter.ErrNotSupported
}
func (c *contextAdapter) ReadTranscript(ref adapter.SessionRef) ([]adapter.TranscriptEvent, error) {
	return nil, adapter.ErrNotSupported
}
func (c *contextAdapter) ContextUsage(ref adapter.SessionRef) (used, limit int, ok bool) {
	return c.used, c.limit, true
}

func TestTrackContextPublishesEvents(t *testing.T) {
	st := newStore(t)
	b := bus.New()
	m := New(st, b)
	p, _ := st.CreateProject("App", "")
	sess, _ := st.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "fake-ctx", Mode: "wrapper"})

	// Threshold 80, context 81/100 → deve emitir session.context e session.context_high
	_ = st.SetSetting("handoff_threshold_pct", "80")

	ca := &contextAdapter{used: 81, limit: 100}
	ref := adapter.SessionRef{Adapter: "fake-ctx"}

	allEvents, cancel := b.Subscribe()
	defer cancel()

	contextEvents := make(chan bus.Event, 10)
	highEvents := make(chan bus.Event, 10)
	go func() {
		for e := range allEvents {
			if e.Type == "session.context" {
				contextEvents <- e
			}
			if e.Type == "session.context_high" {
				highEvents <- e
			}
		}
	}()

	m.trackContext(sess.ID, ref, ca)

	select {
	case <-contextEvents:
	case <-time.After(time.Second):
		t.Fatal("session.context não publicado")
	}
	select {
	case <-highEvents:
	case <-time.After(time.Second):
		t.Fatal("session.context_high não publicado")
	}

	// Segunda chamada NÃO deve re-publicar session.context_high
	m.trackContext(sess.ID, ref, ca)
	select {
	case <-highEvents:
		t.Fatal("session.context_high publicado duas vezes")
	case <-time.After(100 * time.Millisecond):
		// ok — não re-publicou
	}
}

func TestDeriveTitlePicksFirstUserMessage(t *testing.T) {
	events := []adapter.TranscriptEvent{
		{Role: "assistant", Content: "ok"},
		{Role: "user", Content: "Ajustar o tema do terminal para acompanhar a página"},
	}
	got := deriveTitle(events)
	want := "Ajustar o tema do terminal para acompanhar a página"
	if got != want {
		t.Fatalf("deriveTitle = %q, quer %q", got, want)
	}
}
