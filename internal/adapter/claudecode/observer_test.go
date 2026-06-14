package claudecode

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
)

// Fixture: 4 linhas JSONL reais (formato derivado de ~/.claude/projects).
const fixtureJSONL = `{"type":"user","sessionId":"sess-abc","cwd":"/tmp/proj-x","gitBranch":"main","timestamp":"2026-06-12T10:00:00Z","uuid":"u1","message":{"role":"user","content":"como faço deploy?"}}
{"type":"assistant","sessionId":"sess-abc","cwd":"/tmp/proj-x","timestamp":"2026-06-12T10:00:05Z","uuid":"a1","message":{"role":"assistant","model":"claude","usage":{"input_tokens":6,"output_tokens":190,"cache_read_input_tokens":16782},"content":[{"type":"thinking","thinking":"hmm"},{"type":"text","text":"Rode make deploy."}]}}
{"type":"attachment","sessionId":"sess-abc","timestamp":"2026-06-12T10:00:06Z","uuid":"x1"}
{"type":"ai-title","sessionId":"sess-abc","title":"Deploy do projeto X"}`

func writeFixture(t *testing.T) (root, jsonlPath string) {
	t.Helper()
	root = t.TempDir()
	// estrutura: <root>/<dir-encoded>/<session-id>.jsonl
	d := filepath.Join(root, "-tmp-proj-x")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	jsonlPath = filepath.Join(d, "sess-abc.jsonl")
	if err := os.WriteFile(jsonlPath, []byte(fixtureJSONL), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, jsonlPath
}

func TestDiscoverSessions(t *testing.T) {
	root, _ := writeFixture(t)
	a := &Adapter{ProjectsRoot: root}
	sessions, err := a.DiscoverSessions(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	s := sessions[0]
	if s.ExternalRef != "sess-abc" || s.Dir != "/tmp/proj-x" {
		t.Fatalf("got %+v", s)
	}
	if s.Title != "Deploy do projeto X" {
		t.Fatalf("title = %q", s.Title)
	}
}

// writeSession grava um jsonl com um único 1º prompt de usuário arbitrário.
func writeSession(t *testing.T, root, dirEnc, sessID, firstUser string) {
	t.Helper()
	d := filepath.Join(root, dirEnc)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"user","sessionId":"` + sessID + `","cwd":"/tmp/p","timestamp":"2026-06-12T10:00:00Z","message":{"role":"user","content":` + jsonStr(firstUser) + `}}`
	if err := os.WriteFile(filepath.Join(d, sessID+".jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
}

func jsonStr(s string) string {
	b := []byte{'"'}
	for _, r := range s {
		switch r {
		case '"':
			b = append(b, '\\', '"')
		case '\\':
			b = append(b, '\\', '\\')
		case '\n':
			b = append(b, '\\', 'n')
		default:
			b = append(b, string(r)...)
		}
	}
	return string(append(b, '"'))
}

// TestDiscoverSkipsMetaSessions: meta-sessões do worrel (1º prompt = assinatura
// do destilador, com e sem aspas) são descartadas na descoberta; sessões reais
// permanecem. Garante inventário == importador.
func TestDiscoverSkipsMetaSessions(t *testing.T) {
	root := t.TempDir()
	writeSession(t, root, "-tmp-meta1", "meta-plain", "Você é um destilador de conhecimento. Resuma.")
	writeSession(t, root, "-tmp-meta2", "meta-quoted", `"Você é um destilador de conhecimento. Resuma."`)
	writeSession(t, root, "-tmp-real1", "real-1", "como faço deploy?")
	writeSession(t, root, "-tmp-real2", "real-2", "corrija este bug em Go")

	a := &Adapter{ProjectsRoot: root}
	got, err := a.DiscoverSessions(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("sessions = %d, want 2 (meta descartadas)", len(got))
	}
	for _, s := range got {
		if s.ExternalRef == "meta-plain" || s.ExternalRef == "meta-quoted" {
			t.Fatalf("meta-sessão %q não foi descartada", s.ExternalRef)
		}
	}
}

func TestDiscoverSessionsSince(t *testing.T) {
	root, jsonlPath := writeFixture(t)
	future := time.Now().Add(time.Hour)
	os.Chtimes(jsonlPath, future, future)
	a := &Adapter{ProjectsRoot: root}
	// since no futuro distante → nada
	got, _ := a.DiscoverSessions(future.Add(time.Hour))
	if len(got) != 0 {
		t.Fatalf("esperava 0 com since futuro, got %d", len(got))
	}
}

func TestReadTranscriptNormalizes(t *testing.T) {
	_, jsonlPath := writeFixture(t)
	a := &Adapter{}
	evs, err := a.ReadTranscript(adapterRef("sess-abc", jsonlPath))
	if err != nil {
		t.Fatal(err)
	}
	// user + assistant (attachment/ai-title ignorados)
	if len(evs) != 2 {
		t.Fatalf("eventos = %d, want 2: %+v", len(evs), evs)
	}
	if evs[0].Role != "user" || evs[0].Content != "como faço deploy?" {
		t.Fatalf("ev0 = %+v", evs[0])
	}
	if evs[1].Role != "assistant" || evs[1].TokensOut != 190 {
		t.Fatalf("ev1 = %+v", evs[1])
	}
	// thinking + text concatenados
	if !contains(evs[1].Content, "Rode make deploy.") {
		t.Fatalf("conteúdo assistant = %q", evs[1].Content)
	}
}

func adapterRef(ref, path string) adapter.SessionRef {
	return adapter.SessionRef{Adapter: "claude-code", ExternalRef: ref, Path: path}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (indexOf(s, sub) >= 0) }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
