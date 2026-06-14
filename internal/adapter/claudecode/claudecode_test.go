package claudecode

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
)

func TestID(t *testing.T) {
	if New().ID() != "claude-code" {
		t.Fatalf("id = %q", New().ID())
	}
}

func TestBuildInteractive(t *testing.T) {
	a := New()
	spec, err := a.BuildInteractive(adapter.SpawnOpts{
		SessionID:    "11111111-1111-1111-1111-111111111111",
		WorkingDir:   "/tmp/proj",
		Primer:       "# Memória do projeto\nUse tabs.",
		SystemAppend: "Quando concluir uma tarefa, chame report_task_completed.",
		MCPURL:       "http://127.0.0.1:7717/mcp?s=tok123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Path != "claude" {
		t.Fatalf("path = %q", spec.Path)
	}
	if spec.Dir != "/tmp/proj" {
		t.Fatalf("dir = %q", spec.Dir)
	}
	args := spec.Args
	// --session-id <uuid>
	i := slices.Index(args, "--session-id")
	if i < 0 || args[i+1] != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("--session-id ausente/errado: %v", args)
	}
	// --append-system-prompt com o texto de auto-relato
	j := slices.Index(args, "--append-system-prompt")
	if j < 0 || !strings.Contains(args[j+1], "report_task_completed") {
		t.Fatalf("--append-system-prompt ausente: %v", args)
	}
	// --mcp-config com JSON inline contendo a URL e nome worrel
	k := slices.Index(args, "--mcp-config")
	if k < 0 || !strings.Contains(args[k+1], "http://127.0.0.1:7717/mcp?s=tok123") ||
		!strings.Contains(args[k+1], "\"worrel\"") {
		t.Fatalf("--mcp-config ausente/errado: %v", args)
	}
	// primer como ÚLTIMO argumento posicional (visível no transcript),
	// precedido por "--" para evitar consumo pelo parser variádico de --mcp-config.
	if args[len(args)-2] != "--" || args[len(args)-1] != "# Memória do projeto\nUse tabs." {
		t.Fatalf("primer não é o último arg ou falta '--': %v", args)
	}
}

func TestBuildInteractiveNoPrimer(t *testing.T) {
	spec, err := New().BuildInteractive(adapter.SpawnOpts{SessionID: "x"})
	if err != nil {
		t.Fatal(err)
	}
	// sem primer, nenhum arg vazio
	if slices.Contains(spec.Args, "") {
		t.Fatalf("arg vazio indevido: %v", spec.Args)
	}
}

// fixtureJSONL imita o transcript JSONL real do Claude Code: entradas de
// usuário sem usage e mensagens de assistente com message.usage.
const fixtureUsageJSONL = `{"type":"user","message":{"role":"user","content":"oi"}}
{"type":"assistant","message":{"role":"assistant","usage":{"input_tokens":100,"cache_creation_input_tokens":200,"cache_read_input_tokens":300,"output_tokens":50}}}
{"type":"user","message":{"role":"user","content":"continua"}}
{"type":"assistant","message":{"role":"assistant","usage":{"input_tokens":1000,"cache_creation_input_tokens":2000,"cache_read_input_tokens":3000,"output_tokens":500}}}
`

func TestContextUsageFromJSONLPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sess.jsonl")
	if err := os.WriteFile(path, []byte(fixtureUsageJSONL), 0o600); err != nil {
		t.Fatal(err)
	}

	a := New()
	used, limit, ok := a.ContextUsage(adapter.SessionRef{Path: path})
	if !ok {
		t.Fatal("ok=false com fixture válida")
	}
	// ÚLTIMA mensagem com usage: 1000+2000+3000+500 = 6500
	if used != 6500 {
		t.Fatalf("used = %d, want 6500 (última mensagem, não a primeira)", used)
	}
	if limit != contextWindowTokens {
		t.Fatalf("limit = %d, want %d", limit, contextWindowTokens)
	}
}

func TestContextUsageResolvesViaProjectsRoot(t *testing.T) {
	root := t.TempDir()
	projDir := filepath.Join(root, "-Users-x-proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ref := "11111111-2222-3333-4444-555555555555"
	if err := os.WriteFile(filepath.Join(projDir, ref+".jsonl"), []byte(fixtureUsageJSONL), 0o600); err != nil {
		t.Fatal(err)
	}

	a := &Adapter{ProjectsRoot: root}
	used, _, ok := a.ContextUsage(adapter.SessionRef{ExternalRef: ref})
	if !ok || used != 6500 {
		t.Fatalf("resolução via projectsRoot: used=%d ok=%v", used, ok)
	}
}

func TestContextUsageMissingFile(t *testing.T) {
	a := &Adapter{ProjectsRoot: t.TempDir()}
	if _, _, ok := a.ContextUsage(adapter.SessionRef{ExternalRef: "nao-existe"}); ok {
		t.Fatal("ok=true para sessão inexistente")
	}
	if _, _, ok := a.ContextUsage(adapter.SessionRef{}); ok {
		t.Fatal("ok=true para ref vazia")
	}
}

func TestContextUsageNoUsageData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	_ = os.WriteFile(path, []byte(`{"type":"user","message":{"role":"user","content":"oi"}}`+"\n"), 0o600)
	if _, _, ok := New().ContextUsage(adapter.SessionRef{Path: path}); ok {
		t.Fatal("ok=true sem nenhuma entrada com usage")
	}
}

func TestExtractHeadlessResult(t *testing.T) {
	arr := `[{"type":"system","subtype":"init"},{"type":"assistant"},{"type":"result","result":"[{\"type\":\"skill.learned\"}]"}]`
	if got := extractHeadlessResult([]byte(arr)); got != `[{"type":"skill.learned"}]` {
		t.Fatalf("array envelope: %q", got)
	}
	obj := `{"type":"result","result":"texto final"}`
	if got := extractHeadlessResult([]byte(obj)); got != "texto final" {
		t.Fatalf("object envelope: %q", got)
	}
	raw := "não é json"
	if got := extractHeadlessResult([]byte(raw)); got != raw {
		t.Fatalf("raw passthrough: %q", got)
	}
}

func TestBuildRunArgsModel(t *testing.T) {
	args := buildRunArgs("oi", adapter.HeadlessOpts{})
	if slices.Contains(args, "--model") {
		t.Fatalf("sem Model não deve incluir --model: %v", args)
	}
	args = buildRunArgs("oi", adapter.HeadlessOpts{Model: "claude-sonnet-4-6"})
	i := slices.Index(args, "--model")
	if i < 0 || i+1 >= len(args) || args[i+1] != "claude-sonnet-4-6" {
		t.Fatalf("--model <model> ausente/incorreto: %v", args)
	}
}
