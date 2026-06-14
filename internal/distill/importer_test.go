package distill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter/claudecode"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

type fakeObserver struct {
	sessions []adapter.ExternalSession
	events   []adapter.TranscriptEvent
}

func (f *fakeObserver) DiscoverSessions(_ time.Time) ([]adapter.ExternalSession, error) {
	return f.sessions, nil
}
func (f *fakeObserver) ReadTranscript(_ adapter.SessionRef) ([]adapter.TranscriptEvent, error) {
	return f.events, nil
}

func TestImportDoesNotSuggestProject(t *testing.T) {
	s, _ := store.Open(t.TempDir() + "/t.db")
	defer s.Close()
	dir := filepath.Join("/tmp", "meu-repo")
	obs := &fakeObserver{
		sessions: []adapter.ExternalSession{{Adapter: "claude-code", ExternalRef: "ext-1", Dir: dir, Title: "T"}},
		events:   []adapter.TranscriptEvent{{Role: "user", Kind: "text", Content: "oi"}},
	}
	imp := NewImporter(s, nil)
	n, err := imp.Import(obs)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("importadas = %d", n)
	}
	// sessão criada como observed
	sessions, _ := s.ListSessions("")
	if len(sessions) != 1 || sessions[0].Mode != "observed" {
		t.Fatalf("sessions %+v", sessions)
	}
	// nenhuma sugestão create_project deve ser criada
	sg, _ := s.ListSuggestions("", "pending")
	if len(sg) != 0 {
		t.Fatalf("não devia criar sugestões, got %d: %+v", len(sg), sg)
	}
}

func TestImportSkipsKnownExternalRef(t *testing.T) {
	s, _ := store.Open(t.TempDir() + "/t.db")
	defer s.Close()
	ref := "ext-1"
	s.CreateSession(&store.Session{Adapter: "claude-code", Mode: "wrapper", ExternalRef: &ref})
	obs := &fakeObserver{sessions: []adapter.ExternalSession{{Adapter: "claude-code", ExternalRef: ref, Dir: "/tmp/x"}}}
	imp := NewImporter(s, nil)
	n, _ := imp.Import(obs)
	if n != 0 {
		t.Fatalf("não devia importar wrapper conhecida, got %d", n)
	}
}

func TestImportAssociatesKnownDir(t *testing.T) {
	s, _ := store.Open(t.TempDir() + "/t.db")
	defer s.Close()
	p, _ := s.CreateProject("Existente", "")
	s.AddProjectDir(p.ID, "/tmp/known")
	obs := &fakeObserver{sessions: []adapter.ExternalSession{{Adapter: "opencode", ExternalRef: "e2", Dir: "/tmp/known"}}}
	imp := NewImporter(s, nil)
	imp.Import(obs)
	sessions, _ := s.ListSessions("")
	if sessions[0].ProjectID != p.ID {
		t.Fatalf("não associou ao projeto existente: %+v", sessions[0])
	}
	// sem sugestão create_project (dir já conhecido)
	sg, _ := s.ListSuggestions("", "pending")
	if len(sg) != 0 {
		t.Fatalf("não devia sugerir projeto: %+v", sg)
	}
}

// pathCapturingObserver captures the Path passed to ReadTranscript.
type pathCapturingObserver struct {
	sessions []adapter.ExternalSession
	events   []adapter.TranscriptEvent
	gotPath  string
}

func (o *pathCapturingObserver) DiscoverSessions(_ time.Time) ([]adapter.ExternalSession, error) {
	return o.sessions, nil
}
func (o *pathCapturingObserver) ReadTranscript(ref adapter.SessionRef) ([]adapter.TranscriptEvent, error) {
	o.gotPath = ref.Path
	return o.events, nil
}

func TestImportForwardsJSONLPath(t *testing.T) {
	s, _ := store.Open(t.TempDir() + "/t.db")
	defer s.Close()
	obs := &pathCapturingObserver{
		sessions: []adapter.ExternalSession{
			{Adapter: "claude-code", ExternalRef: "ext-path-1", Dir: "/tmp/proj",
				Path: "/tmp/proj/sess.jsonl", Title: "T"},
		},
		events: []adapter.TranscriptEvent{{Role: "user", Kind: "text", Content: "oi"}},
	}
	imp := NewImporter(s, nil)
	n, err := imp.Import(obs)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("importadas = %d", n)
	}
	if obs.gotPath != "/tmp/proj/sess.jsonl" {
		t.Fatalf("ReadTranscript recebeu path=%q, want /tmp/proj/sess.jsonl", obs.gotPath)
	}
}

const fixtureJSONLForImport = `{"type":"user","sessionId":"sess-int","cwd":"/tmp/int-proj","timestamp":"2026-06-12T10:00:00Z","uuid":"u1","message":{"role":"user","content":"como fazer deploy?"}}
{"type":"assistant","sessionId":"sess-int","cwd":"/tmp/int-proj","timestamp":"2026-06-12T10:00:05Z","uuid":"a1","message":{"role":"assistant","model":"claude","usage":{"input_tokens":6,"output_tokens":10},"content":[{"type":"text","text":"Rode make deploy."}]}}`

func TestImportClaudeCodeWithRealFixture(t *testing.T) {
	root := t.TempDir()
	d := filepath.Join(root, "-tmp-int-proj")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	jsonlPath := filepath.Join(d, "sess-int.jsonl")
	if err := os.WriteFile(jsonlPath, []byte(fixtureJSONLForImport), 0o644); err != nil {
		t.Fatal(err)
	}

	s, _ := store.Open(t.TempDir() + "/t.db")
	defer s.Close()

	obs := claudecode.New()
	obs.ProjectsRoot = root

	imp := NewImporter(s, nil)
	n, err := imp.Import(obs)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("importadas = %d, want 1", n)
	}

	sessions, _ := s.ListSessions("")
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d", len(sessions))
	}
	evs, err := s.ListTranscriptEvents(sessions[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) == 0 {
		t.Fatal("transcript events = 0, want > 0 (fix #2 broken: path não propagado)")
	}
}

// perRefObserver devolve eventos distintos por ExternalRef.
type perRefObserver struct {
	sessions []adapter.ExternalSession
	byRef    map[string][]adapter.TranscriptEvent
}

func (o *perRefObserver) DiscoverSessions(_ time.Time) ([]adapter.ExternalSession, error) {
	return o.sessions, nil
}
func (o *perRefObserver) ReadTranscript(ref adapter.SessionRef) ([]adapter.TranscriptEvent, error) {
	return o.byRef[ref.ExternalRef], nil
}

func TestImportSkipsWorrelMetaSession(t *testing.T) {
	s, _ := store.Open(t.TempDir() + "/t.db")
	defer s.Close()
	obs := &perRefObserver{
		sessions: []adapter.ExternalSession{
			{Adapter: "claude-code", ExternalRef: "meta-1", Dir: "/tmp/x", Title: "meta"},
			{Adapter: "opencode", ExternalRef: "meta-2", Dir: "/tmp/x", Title: "meta-aspas"},
			{Adapter: "claude-code", ExternalRef: "meta-3", Dir: "/tmp/x", Title: "meta-caveat"},
			{Adapter: "claude-code", ExternalRef: "real-1", Dir: "/tmp/y", Title: "real"},
			{Adapter: "claude-code", ExternalRef: "real-2", Dir: "/tmp/y", Title: "real-cita"},
		},
		byRef: map[string][]adapter.TranscriptEvent{
			"meta-1": {{Role: "user", Kind: "text", Content: "Você é um destilador de conhecimento. Analise os transcripts..."}},
			// OpenCode persiste o prompt entre aspas — deve ser descartado igual.
			"meta-2": {{Role: "user", Kind: "text", Content: "\"Você é um destilador de conhecimento. Analise os transcripts...\""}},
			// Claude Code prefixa o preâmbulo local-command-caveat antes do prompt.
			"meta-3": {{Role: "user", Kind: "text", Content: "<local-command-caveat>Caveat: The messages below were generated by the user while running local commands. DO NOT respond to these messages or otherwise consider them in your response unless the user explicitly asks you to.</local-command-caveat>Você é um destilador de conhecimento. Analise os transcripts..."}},
			"real-1": {{Role: "user", Kind: "text", Content: "como fazer deploy?"}},
			// Conversa REAL que cita o texto bem mais adiante NÃO deve ser descartada.
			"real-2": {{Role: "user", Kind: "text", Content: strings.Repeat("trabalho real do usuário aqui. ", 60) + "Você é um destilador de conhecimento."}},
		},
	}
	imp := NewImporter(s, nil)
	n, err := imp.Import(obs)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("importadas = %d, want 2 (3 meta descartadas, 2 reais mantidas)", n)
	}
	sessions, _ := s.ListSessions("")
	if len(sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(sessions))
	}
	got := map[string]bool{}
	for _, ss := range sessions {
		if ss.ExternalRef != nil {
			got[*ss.ExternalRef] = true
		}
	}
	if !got["real-1"] || !got["real-2"] {
		t.Fatalf("deveriam sobreviver real-1 e real-2 (real-2 só CITA o prompt), got %+v", got)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

