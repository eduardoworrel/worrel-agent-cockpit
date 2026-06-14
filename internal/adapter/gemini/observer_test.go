package gemini

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
)

// setupProjectDir cria um diretório de projeto fake do Gemini CLI dentro de um
// tmpRoot, com .project_root, logs.json e (opcionalmente) um chat rico.
func setupProjectDir(t *testing.T, id, projectRoot string) (root, dir string) {
	t.Helper()
	root = t.TempDir()
	dir = filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".project_root"), []byte(projectRoot), 0o644); err != nil {
		t.Fatal(err)
	}
	logs := `[
	  {"sessionId":"s1","messageId":0,"timestamp":"2026-01-01T10:00:00.000Z","type":"user","message":"first prompt"},
	  {"sessionId":"s1","messageId":1,"timestamp":"2026-01-01T10:05:00.000Z","type":"user","message":"second prompt"}
	]`
	if err := os.WriteFile(filepath.Join(dir, "logs.json"), []byte(logs), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, dir
}

func TestDiscoverSessions(t *testing.T) {
	root, dir := setupProjectDir(t, "myproj-abc", "/home/u/myproj")
	a := &Adapter{TmpRoot: root}
	sessions, err := a.DiscoverSessions(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	s := sessions[0]
	if s.Adapter != "gemini" {
		t.Errorf("adapter = %q", s.Adapter)
	}
	if s.ExternalRef != "myproj-abc" {
		t.Errorf("externalRef = %q", s.ExternalRef)
	}
	if s.Dir != "/home/u/myproj" {
		t.Errorf("dir = %q", s.Dir)
	}
	if s.Path != dir {
		t.Errorf("path = %q, want %q", s.Path, dir)
	}
	if s.Title != "first prompt" {
		t.Errorf("title = %q", s.Title)
	}
	if s.StartedAt.IsZero() || s.UpdatedAt.IsZero() {
		t.Errorf("times not set: %v %v", s.StartedAt, s.UpdatedAt)
	}
	if !s.UpdatedAt.After(s.StartedAt) {
		t.Errorf("updated should be after started")
	}
}

func TestDiscoverSessionsSinceFilter(t *testing.T) {
	root, _ := setupProjectDir(t, "p1", "/x")
	a := &Adapter{TmpRoot: root}
	// since in the future -> filtered out
	sessions, err := a.DiscoverSessions(time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Errorf("future since should filter all, got %d", len(sessions))
	}
}

func TestDiscoverSessionsMissingRoot(t *testing.T) {
	a := &Adapter{TmpRoot: filepath.Join(t.TempDir(), "nope")}
	sessions, err := a.DiscoverSessions(time.Time{})
	if err != nil {
		t.Fatalf("missing root should not error: %v", err)
	}
	if sessions != nil {
		t.Errorf("want nil, got %v", sessions)
	}
}

func TestReadTranscriptRichHistory(t *testing.T) {
	_, dir := setupProjectDir(t, "p", "/x")
	chats := filepath.Join(dir, "chats")
	if err := os.MkdirAll(chats, 0o755); err != nil {
		t.Fatal(err)
	}
	hist := `{"history":[
	  {"role":"user","parts":[{"text":"what is 2+2?"}]},
	  {"role":"model","parts":[{"text":"4"}]}
	],"authType":"oauth"}`
	if err := os.WriteFile(filepath.Join(chats, "checkpoint-foo.json"), []byte(hist), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	events, err := a.ReadTranscript(adapter.SessionRef{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].Role != "user" || events[0].Content != "what is 2+2?" {
		t.Errorf("event0 = %+v", events[0])
	}
	if events[1].Role != "assistant" || events[1].Content != "4" {
		t.Errorf("event1 = %+v (model should map to assistant)", events[1])
	}
}

func TestReadTranscriptBareArray(t *testing.T) {
	_, dir := setupProjectDir(t, "p", "/x")
	chats := filepath.Join(dir, "checkpoints")
	if err := os.MkdirAll(chats, 0o755); err != nil {
		t.Fatal(err)
	}
	hist := `[{"role":"user","parts":[{"text":"hi"}]},{"role":"model","parts":[{"text":"hello"}]}]`
	if err := os.WriteFile(filepath.Join(chats, "checkpoint-x.json"), []byte(hist), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New()
	events, err := a.ReadTranscript(adapter.SessionRef{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Content != "hello" {
		t.Fatalf("bare array parse failed: %+v", events)
	}
}

func TestReadTranscriptFallbackLogs(t *testing.T) {
	_, dir := setupProjectDir(t, "p", "/x")
	// no chats/checkpoints -> degrade to logs.json (user prompts only)
	a := New()
	events, err := a.ReadTranscript(adapter.SessionRef{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	for _, e := range events {
		if e.Role != "user" {
			t.Errorf("logs fallback should be all user, got %q", e.Role)
		}
	}
	if events[0].Content != "first prompt" {
		t.Errorf("event0 = %q", events[0].Content)
	}
}

func TestReadTranscriptNoPath(t *testing.T) {
	a := New()
	if _, err := a.ReadTranscript(adapter.SessionRef{}); err != adapter.ErrNotSupported {
		t.Errorf("empty path should return ErrNotSupported, got %v", err)
	}
}

func TestContextUsageDegrades(t *testing.T) {
	a := New()
	if _, _, ok := a.ContextUsage(adapter.SessionRef{Path: "/anything"}); ok {
		t.Errorf("ContextUsage should degrade (ok=false)")
	}
}
