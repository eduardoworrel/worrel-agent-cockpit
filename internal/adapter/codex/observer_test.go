package codex

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
)

// Fixture: rollout JSONL no formato real do Codex (cli_version 0.116).
const fixtureRollout = `{"timestamp":"2026-03-25T14:58:00.836Z","type":"session_meta","payload":{"id":"019d2580-96dd-7513-8d53-9714572c18ea","timestamp":"2026-03-25T14:57:57.479Z","cwd":"/tmp/proj-x","originator":"codex_exec","cli_version":"0.116.0"}}
{"timestamp":"2026-03-25T14:58:00.837Z","type":"event_msg","payload":{"type":"task_started","model_context_window":258400}}
{"timestamp":"2026-03-25T14:58:00.837Z","type":"task_started","payload":{"type":"task_started","model_context_window":258400}}
{"timestamp":"2026-03-25T14:58:00.838Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"<permissions instructions>secret</permissions instructions>"}]}}
{"timestamp":"2026-03-25T14:58:00.839Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context><cwd>/tmp/proj-x</cwd></environment_context>"}]}}
{"timestamp":"2026-03-25T14:58:01.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"como faço deploy?"}]}}
{"timestamp":"2026-03-25T14:59:47.616Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Rode make deploy."}]}}
{"timestamp":"2026-03-25T14:59:48.000Z","type":"token_count","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":999000,"output_tokens":1000,"total_tokens":1000000},"last_token_usage":{"input_tokens":10632,"output_tokens":136,"total_tokens":10768}}}}`

func writeRollout(t *testing.T) (root, path string) {
	t.Helper()
	root = t.TempDir()
	d := filepath.Join(root, "2026", "03", "25")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(d, "rollout-2026-03-25T11-57-57-019d2580.jsonl")
	if err := os.WriteFile(path, []byte(fixtureRollout), 0o644); err != nil {
		t.Fatal(err)
	}
	// arquivo .jsonl que NÃO é rollout → deve ser ignorado.
	if err := os.WriteFile(filepath.Join(d, "history.jsonl"), []byte(`{"x":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, path
}

func TestDiscoverSessions(t *testing.T) {
	root, _ := writeRollout(t)
	a := &Adapter{SessionsRoot: root}
	sessions, err := a.DiscoverSessions(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("esperava 1 sessão (rollout), veio %d", len(sessions))
	}
	s := sessions[0]
	if s.Adapter != "codex" {
		t.Errorf("Adapter = %q", s.Adapter)
	}
	if s.ExternalRef != "019d2580-96dd-7513-8d53-9714572c18ea" {
		t.Errorf("ExternalRef = %q", s.ExternalRef)
	}
	if s.Dir != "/tmp/proj-x" {
		t.Errorf("Dir = %q", s.Dir)
	}
	if s.Title != "como faço deploy?" {
		t.Errorf("Title = %q (deveria pular msgs sintéticas <...>)", s.Title)
	}
	if s.StartedAt.IsZero() || s.UpdatedAt.IsZero() {
		t.Error("tempos não preenchidos")
	}
	if !s.UpdatedAt.After(s.StartedAt) {
		t.Error("UpdatedAt deveria ser depois de StartedAt")
	}
}

func TestDiscoverSessionsSinceFilter(t *testing.T) {
	root, _ := writeRollout(t)
	a := &Adapter{SessionsRoot: root}
	future := time.Now().Add(24 * time.Hour)
	sessions, err := a.DiscoverSessions(future)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("filtro since futuro deveria retornar 0, veio %d", len(sessions))
	}
}

func TestReadTranscript(t *testing.T) {
	_, path := writeRollout(t)
	a := New()
	events, err := a.ReadTranscript(adapter.SessionRef{Adapter: "codex", Path: path})
	if err != nil {
		t.Fatal(err)
	}
	// developer + user sintético devem ser pulados → só user real + assistant.
	if len(events) != 2 {
		t.Fatalf("esperava 2 eventos, veio %d: %+v", len(events), events)
	}
	if events[0].Role != "user" || events[0].Content != "como faço deploy?" {
		t.Errorf("evento[0] = %+v", events[0])
	}
	if events[1].Role != "assistant" || events[1].Content != "Rode make deploy." {
		t.Errorf("evento[1] = %+v", events[1])
	}
	if events[1].Kind != "text" {
		t.Errorf("Kind = %q", events[1].Kind)
	}
	if events[0].CreatedAt == 0 {
		t.Error("CreatedAt não preenchido")
	}
}

func TestContextUsage(t *testing.T) {
	_, path := writeRollout(t)
	used, limit, ok := New().ContextUsage(adapter.SessionRef{Path: path})
	if !ok {
		t.Fatal("ok deveria ser true")
	}
	if used != 10768 {
		t.Errorf("used = %d, quer 10768", used)
	}
	if limit != 258400 {
		t.Errorf("limit = %d, quer 258400 (model_context_window)", limit)
	}
}

func TestContextUsageNoPath(t *testing.T) {
	if _, _, ok := New().ContextUsage(adapter.SessionRef{}); ok {
		t.Error("sem path, ok deveria ser false")
	}
}

func TestDiscoverSessionsMissingRoot(t *testing.T) {
	a := &Adapter{SessionsRoot: filepath.Join(t.TempDir(), "nao-existe")}
	sessions, err := a.DiscoverSessions(time.Time{})
	if err != nil {
		t.Fatalf("root inexistente não deveria dar erro: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("esperava 0 sessões, veio %d", len(sessions))
	}
}
