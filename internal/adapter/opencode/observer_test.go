package opencode

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	_ "modernc.org/sqlite"
)

// buildFixtureDB cria um opencode.db mínimo com o schema real (subconjunto).
func buildFixtureDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "opencode.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	schema := `
	CREATE TABLE project (id TEXT PRIMARY KEY, worktree TEXT NOT NULL, name TEXT, time_created INTEGER, time_updated INTEGER);
	CREATE TABLE session (id TEXT PRIMARY KEY, project_id TEXT, directory TEXT NOT NULL, title TEXT NOT NULL,
		time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL, tokens_input INTEGER DEFAULT 0, tokens_output INTEGER DEFAULT 0);
	CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, data TEXT NOT NULL);
	CREATE TABLE part (id TEXT PRIMARY KEY, message_id TEXT NOT NULL, session_id TEXT NOT NULL, data TEXT NOT NULL);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	db.Exec(`INSERT INTO project VALUES ('p1','/tmp/oc-proj','OC Proj',1000,2000)`)
	db.Exec(`INSERT INTO session VALUES ('s1','p1','/tmp/oc-proj','Refatorar auth',1000,2000,42,99)`)
	db.Exec(`INSERT INTO message VALUES ('m1','s1',1100,'{"role":"user","time":{"created":1100}}')`)
	db.Exec(`INSERT INTO message VALUES ('m2','s1',1200,'{"role":"assistant","tokens":{"input":42,"output":99},"time":{"created":1200}}')`)
	db.Exec(`INSERT INTO part VALUES ('pt1','m1','s1','{"type":"text","text":"refatore o login"}')`)
	db.Exec(`INSERT INTO part VALUES ('pt2','m2','s1','{"type":"reasoning","text":"vou pensar"}')`)
	db.Exec(`INSERT INTO part VALUES ('pt3','m2','s1','{"type":"text","text":"feito, veja diff"}')`)
	db.Exec(`INSERT INTO part VALUES ('pt4','m2','s1','{"type":"step-finish"}')`)
	return path
}

func TestOpenCodeDiscover(t *testing.T) {
	a := &Adapter{DBPath: buildFixtureDB(t)}
	sessions, err := a.DiscoverSessions(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d", len(sessions))
	}
	if sessions[0].ExternalRef != "s1" || sessions[0].Dir != "/tmp/oc-proj" || sessions[0].Title != "Refatorar auth" {
		t.Fatalf("got %+v", sessions[0])
	}
}

func TestOpenCodeReadTranscript(t *testing.T) {
	a := &Adapter{DBPath: buildFixtureDB(t)}
	evs, err := a.ReadTranscript(refOC("s1"))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 {
		t.Fatalf("eventos = %d: %+v", len(evs), evs)
	}
	if evs[0].Role != "user" || evs[0].Content != "refatore o login" {
		t.Fatalf("ev0 %+v", evs[0])
	}
	if evs[1].Role != "assistant" || evs[1].TokensOut != 99 {
		t.Fatalf("ev1 %+v", evs[1])
	}
	if !contains(evs[1].Content, "feito, veja diff") {
		t.Fatalf("assistant content %q", evs[1].Content)
	}
}

func TestOpenRO_IsReadOnly(t *testing.T) {
	path := buildFixtureDB(t)
	a := &Adapter{DBPath: path}
	db, err := a.openRO()
	if err != nil {
		t.Fatalf("openRO failed: %v", err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO session VALUES ('x','p1','/x','t',1,2,0,0)`)
	if err == nil {
		t.Fatal("expected read-only error on INSERT, got nil")
	}
}

func refOC(ref string) adapter.SessionRef {
	return adapter.SessionRef{Adapter: "opencode", ExternalRef: ref}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
