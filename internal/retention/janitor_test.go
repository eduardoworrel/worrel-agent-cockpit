package retention

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// testStore acopla o store ao caminho do arquivo para permitir que os testes
// abram uma conexão própria (envelhecer sessões sem helper em produção).
type testStore struct {
	*store.Store
	path string
}

func newStore(t *testing.T) testStore {
	t.Helper()
	path := t.TempDir() + "/t.db"
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return testStore{Store: s, path: path}
}

func TestSweepPrunesOnlyExpired(t *testing.T) {
	s := newStore(t)
	p, _ := s.CreateProject("App", "")
	j := New(s.Store)

	old, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	recent, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	_ = s.AppendTranscriptEvent(old.ID, "user", "text", "antigo", 0, 0)
	_ = s.AppendTranscriptEvent(recent.ID, "user", "text", "novo", 0, 0)

	ageSession(t, s, old.ID, 40)

	n, err := j.Sweep()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("podadas = %d, want 1", n)
	}
	if evs, _ := s.ListTranscriptEvents(old.ID); len(evs) != 0 {
		t.Fatal("transcript antigo deveria ter sido podado")
	}
	if evs, _ := s.ListTranscriptEvents(recent.ID); len(evs) != 1 {
		t.Fatal("transcript recente NÃO deveria ter sido podado")
	}
}

func TestSweepRespectsRetentionSetting(t *testing.T) {
	s := newStore(t)
	p, _ := s.CreateProject("App", "")
	_ = s.SetSetting("retention_days", "7")
	j := New(s.Store)

	old, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	_ = s.AppendTranscriptEvent(old.ID, "user", "text", "x", 0, 0)
	ageSession(t, s, old.ID, 10)

	n, _ := j.Sweep()
	if n != 1 {
		t.Fatalf("com retention=7, podadas = %d, want 1", n)
	}
}

func TestSweepIdempotent(t *testing.T) {
	s := newStore(t)
	p, _ := s.CreateProject("App", "")
	j := New(s.Store)
	old, _ := s.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	_ = s.AppendTranscriptEvent(old.ID, "user", "text", "x", 0, 0)
	ageSession(t, s, old.ID, 40)

	if n, _ := j.Sweep(); n != 1 {
		t.Fatalf("1ª varredura = %d", n)
	}
	if n, _ := j.Sweep(); n != 0 {
		t.Fatalf("2ª varredura = %d, want 0 (já podada)", n)
	}
}

// ageSession reescreve datas da sessão para `days` dias atrás (encerrada),
// via conexão própria ao mesmo arquivo SQLite (sem helper de teste em produção).
func ageSession(t *testing.T, s testStore, id string, days int64) {
	t.Helper()
	at := time.Now().UnixMilli() - days*24*60*60*1000
	db, err := sql.Open("sqlite", s.path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE sessions SET status='ended', ended_at=?, started_at=? WHERE id=?`, at, at, id); err != nil {
		t.Fatal(err)
	}
}
