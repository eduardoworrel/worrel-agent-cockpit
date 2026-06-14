package retro

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter/claudecode"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

type fakeObs struct {
	id   string
	sess []adapter.ExternalSession
}

func (f *fakeObs) ID() string { return f.id }
func (f *fakeObs) DiscoverSessions(since time.Time) ([]adapter.ExternalSession, error) {
	var out []adapter.ExternalSession
	for _, s := range f.sess {
		if since.IsZero() || s.UpdatedAt.After(since) {
			out = append(out, s)
		}
	}
	return out, nil
}
func (f *fakeObs) ReadTranscript(ref adapter.SessionRef) ([]adapter.TranscriptEvent, error) {
	return nil, nil
}

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestInventoryNoLLMAndWindow(t *testing.T) {
	s := newStore(t)
	old := time.Now().Add(-100 * 24 * time.Hour)
	recent := time.Now().Add(-10 * 24 * time.Hour)
	obs := &fakeObs{id: "claude-code", sess: []adapter.ExternalSession{
		{Adapter: "claude-code", ExternalRef: "a", Dir: "/repo/x", UpdatedAt: old},
		{Adapter: "claude-code", ExternalRef: "b", Dir: "/repo/x", UpdatedAt: recent},
		{Adapter: "claude-code", ExternalRef: "c", Dir: "/repo/y", UpdatedAt: recent},
	}}
	inv := NewInventory(s, []Observer{obs})

	full, err := inv.Scan(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if full.PerCLI["claude-code"].Sessions != 3 {
		t.Fatalf("total = %d", full.PerCLI["claude-code"].Sessions)
	}
	if len(full.Folders) != 2 {
		t.Fatalf("pastas = %d", len(full.Folders))
	}
	if full.EstimatedInvocations != full.PerCLI["claude-code"].Sessions {
		t.Fatalf("estimativa != sessões: %d", full.EstimatedInvocations)
	}
	// critério 2: janela de 60d reduz proporcionalmente (só 'b' e 'c' são recentes)
	win, _ := inv.Scan(time.Now().Add(-60 * 24 * time.Hour))
	if win.PerCLI["claude-code"].Sessions != 2 {
		t.Fatalf("janela 60d = %d, want 2", win.PerCLI["claude-code"].Sessions)
	}
}

// TestInventorySkipsMetaSessions: o inventário conta o MESMO conjunto que o
// importador. Com 1 meta-sessão do worrel + 2 reais num diretório fake do Claude
// Code, o inventário conta 2 (não 3), pois a meta-sessão é filtrada na descoberta.
func TestInventorySkipsMetaSessions(t *testing.T) {
	s := newStore(t)
	root := t.TempDir()
	writeCCSession(t, root, "-tmp-meta", "meta-1", "Você é um destilador de conhecimento. Resuma.")
	writeCCSession(t, root, "-tmp-r1", "real-1", "como faço deploy?")
	writeCCSession(t, root, "-tmp-r2", "real-2", "corrija este bug")

	obs := &claudecode.Adapter{ProjectsRoot: root}
	inv := NewInventory(s, []Observer{obs})
	r, err := inv.Scan(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if r.PerCLI["claude-code"].Sessions != 2 {
		t.Fatalf("sessions = %d, want 2 (meta descartada)", r.PerCLI["claude-code"].Sessions)
	}
	if r.EstimatedInvocations != 2 {
		t.Fatalf("estimativa = %d, want 2", r.EstimatedInvocations)
	}
}

func writeCCSession(t *testing.T, root, dirEnc, sessID, firstUser string) {
	t.Helper()
	d := filepath.Join(root, dirEnc)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(firstUser)
	line := `{"type":"user","sessionId":"` + sessID + `","cwd":"/tmp/p","timestamp":"2026-06-12T10:00:00Z","message":{"role":"user","content":` + string(b) + `}}`
	if err := os.WriteFile(filepath.Join(d, sessID+".jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInventoryMarksKnown(t *testing.T) {
	s := newStore(t)
	ref := "a"
	_, _ = s.CreateSession(&store.Session{Adapter: "claude-code", Mode: "observed", ExternalRef: &ref})
	obs := &fakeObs{id: "claude-code", sess: []adapter.ExternalSession{
		{Adapter: "claude-code", ExternalRef: "a", Dir: "/x", UpdatedAt: time.Now()},
		{Adapter: "claude-code", ExternalRef: "b", Dir: "/x", UpdatedAt: time.Now()},
	}}
	inv := NewInventory(s, []Observer{obs})
	r, _ := inv.Scan(time.Time{})
	if r.PerCLI["claude-code"].AlreadyKnown != 1 {
		t.Fatalf("known = %d, want 1", r.PerCLI["claude-code"].AlreadyKnown)
	}
	// "a" está importada mas NÃO analisada (analyzed_at NULL) e "b" nem importada:
	// ambas serão processadas pelo run → estimativa = 2 (semântica "não analisadas").
	if r.EstimatedInvocations != 2 {
		t.Fatalf("estimativa = %d, want 2 (não-analisadas)", r.EstimatedInvocations)
	}
	// Ao marcar "a" como analisada, ela sai da estimativa → 1.
	_, _ = s.DB().Exec(`UPDATE sessions SET analyzed_at=? WHERE external_ref=?`, 1, "a")
	r2, _ := inv.Scan(time.Time{})
	if r2.EstimatedInvocations != 1 {
		t.Fatalf("estimativa após analisar 'a' = %d, want 1", r2.EstimatedInvocations)
	}
}
