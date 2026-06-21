package memory_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	eng "github.com/eduardoworrel/worrel-agent-cockpit/internal/engine"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/engine/memory"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

type fakeLLM struct{ out string }

func (f fakeLLM) RunHeadless(_ context.Context, _ string, _ adapter.HeadlessOpts) (string, error) {
	return f.out, nil
}

func seedFriction(t *testing.T, st *store.Store, sessID string) {
	_ = st.AppendTranscriptEventRich(sessID, "assistant", "tool_use", "Bash make build", `[{"type":"tool_use","name":"Bash"}]`, 0, 0)
	_ = st.AppendTranscriptEventRich(sessID, "user", "tool_result", "make: not found", `[{"type":"tool_result","output":"make: not found","is_error":true}]`, 0, 0)
	_ = st.AppendTranscriptEventRich(sessID, "assistant", "tool_use", "Bash go build ./...", `[{"type":"tool_use","name":"Bash"}]`, 0, 0)
}

func TestMemoryEngineHybridCreatesSuggestion(t *testing.T) {
	st, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	p, _ := st.CreateProject("App", "")
	sess, _ := st.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	seedFriction(t, st, sess.ID)

	llm := fakeLLM{out: `[{"content":"build é go build ./...","category":"convencao","evidence":"s1"}]`}
	m := memory.New(llm)

	// modo default hybrid (config vazia → Defaults resolve)
	r := eng.NewRegistry()
	r.Register(m)
	if err := r.Run(context.Background(), st, "memory", p.ID, sess.ID); err != nil {
		t.Fatal(err)
	}
	sgs, _ := st.ListSuggestions("", "")
	if len(sgs) != 1 {
		t.Fatalf("esperava 1 sugestão, got %d", len(sgs))
	}
	if sgs[0].Type != "add_memory_entry" || sgs[0].Origin != "engine:memory" {
		t.Fatalf("type=%q origin=%q", sgs[0].Type, sgs[0].Origin)
	}
	var pl memory.GoldenTruth
	_ = json.Unmarshal([]byte(sgs[0].Payload), &pl)
	if pl.Category != "convencao" {
		t.Fatalf("payload=%+v", pl)
	}
}

func TestMemoryEngineHeuristicOnlyNoLLM(t *testing.T) {
	st, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	p, _ := st.CreateProject("App", "")
	sess, _ := st.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})
	seedFriction(t, st, sess.ID)

	// LLM que explode se for chamado — heuristic_only não deve chamar
	m := memory.New(panicLLM{})
	_ = st.SetEngineConfig("memory", "detection_mode", "heuristic_only", "")

	r := eng.NewRegistry()
	r.Register(m)
	if err := r.Run(context.Background(), st, "memory", p.ID, sess.ID); err != nil {
		t.Fatal(err)
	}
	sgs, _ := st.ListSuggestions("", "")
	if len(sgs) == 0 {
		t.Fatal("heuristic_only deveria gerar ao menos 1 sugestão crua")
	}
}

type panicLLM struct{}

func (panicLLM) RunHeadless(_ context.Context, _ string, _ adapter.HeadlessOpts) (string, error) {
	panic("LLM não deveria ser chamado em heuristic_only")
}

func TestTitleTruncationRuneSafe(t *testing.T) {
	// Build a content string of 90 accented runes (each 'é' is 2 bytes in UTF-8).
	// The heuristic wraps content in a format string, so we seed the transcript
	// directly and let heuristic_only produce a suggestion, then check the title.
	// Because heuristic content is not pure accented text, we test rune-safety
	// via a direct synthetic path: craft a GoldenTruth-like content and verify
	// the same truncation logic the engine applies.
	import_utf8 := func(s string) (valid bool, runeLen int) {
		// inline to avoid import in table; use unicode/utf8 via the std library
		valid = true
		for _, r := range s {
			_ = r
		}
		// check byte-level validity
		b := []byte(s)
		for i := 0; i < len(b); {
			r, size := rune(0), 0
			r = rune(b[i])
			if r < 0x80 {
				size = 1
			} else {
				// multi-byte: rely on Go string range to have been valid already
				size = 1
			}
			_ = r
			i += size
		}
		return valid, len([]rune(s))
	}
	_ = import_utf8 // not used below; use unicode/utf8 directly

	// Use the engine in heuristic_only mode with transcript events whose
	// tool_use content is 50 accented chars each, producing a title > 80 runes.
	accentedCmd := func(n int) string {
		r := make([]rune, n)
		for i := range r {
			r[i] = 'é'
		}
		return string(r)
	}

	st, _ := store.Open(filepath.Join(t.TempDir(), "t.db"))
	p, _ := st.CreateProject("App", "")
	sess, _ := st.CreateSession(&store.Session{ProjectID: p.ID, Adapter: "claude-code", Mode: "wrapper"})

	cmd1 := accentedCmd(50)
	cmd2 := accentedCmd(50)
	_ = st.AppendTranscriptEventRich(sess.ID, "assistant", "tool_use", cmd1, `[{"type":"tool_use","name":"Bash"}]`, 0, 0)
	_ = st.AppendTranscriptEventRich(sess.ID, "user", "tool_result", "erro", `[{"type":"tool_result","output":"erro","is_error":true}]`, 0, 0)
	_ = st.AppendTranscriptEventRich(sess.ID, "assistant", "tool_use", cmd2, `[{"type":"tool_use","name":"Bash"}]`, 0, 0)

	_ = st.SetEngineConfig("memory", "detection_mode", "heuristic_only", "")
	m := memory.New(panicLLM{})
	r := eng.NewRegistry()
	r.Register(m)
	if err := r.Run(context.Background(), st, "memory", p.ID, sess.ID); err != nil {
		t.Fatal(err)
	}
	sgs, _ := st.ListSuggestions("", "")
	if len(sgs) == 0 {
		t.Fatal("esperava ao menos 1 sugestão")
	}
	title := sgs[0].Title
	runeCount := len([]rune(title))
	if runeCount > 80 {
		t.Fatalf("Title excede 80 runas: got %d runas", runeCount)
	}
	// Verify UTF-8 validity by ranging over the string (invalid UTF-8 produces replacement runes).
	var byteLen int
	for _, ch := range title {
		byteLen += len(string(ch))
	}
	if byteLen != len(title) {
		t.Fatalf("Title não é UTF-8 válido")
	}
}
