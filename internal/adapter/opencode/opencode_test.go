package opencode

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
)

func TestID(t *testing.T) {
	if New().ID() != "opencode" {
		t.Fatalf("id = %q", New().ID())
	}
}

func TestBuildInteractiveWritesConfigAndPrompt(t *testing.T) {
	a := New()
	dir := t.TempDir()
	spec, err := a.BuildInteractive(adapter.SpawnOpts{
		WorkingDir: "/tmp/proj",
		Primer:     "primer-text",
		MCPURL:     "http://127.0.0.1:7717/mcp?s=tok",
		ConfigDir:  dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Path != "opencode" {
		t.Fatalf("path = %q", spec.Path)
	}
	// path do projeto como posicional
	if !slices.Contains(spec.Args, "/tmp/proj") {
		t.Fatalf("dir posicional ausente: %v", spec.Args)
	}
	// primer via --prompt
	i := slices.Index(spec.Args, "--prompt")
	if i < 0 || spec.Args[i+1] != "primer-text" {
		t.Fatalf("--prompt ausente: %v", spec.Args)
	}
	// env OPENCODE_CONFIG aponta para arquivo existente com mcp remote
	var cfgPath string
	for _, e := range spec.Env {
		if strings.HasPrefix(e, "OPENCODE_CONFIG=") {
			cfgPath = strings.TrimPrefix(e, "OPENCODE_CONFIG=")
		}
	}
	if cfgPath == "" {
		t.Fatalf("OPENCODE_CONFIG ausente: %v", spec.Env)
	}
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("config não escrito: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	mcp, _ := cfg["mcp"].(map[string]any)
	w, _ := mcp["worrel"].(map[string]any)
	if w["type"] != "remote" || w["url"] != "http://127.0.0.1:7717/mcp?s=tok" {
		t.Fatalf("entrada mcp errada: %v", cfg)
	}
	// Cleanup remove o arquivo
	if spec.Cleanup != nil {
		_ = spec.Cleanup()
		if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
			t.Fatal("Cleanup não removeu config")
		}
	}
}

func TestExtractOpencodeText(t *testing.T) {
	out := `{"type":"step_start","part":{"type":"step-start"}}
{"type":"text","part":{"type":"text","text":"[{\"type\":\"skill.learned\"}]"}}
{"type":"step_finish","part":{"type":"step-finish"}}`
	if got := extractOpencodeText([]byte(out)); got != `[{"type":"skill.learned"}]` {
		t.Fatalf("extração = %q", got)
	}
	// fallback: sem eventos text reconhecíveis devolve cru
	if got := extractOpencodeText([]byte("texto solto")); got != "texto solto" {
		t.Fatalf("fallback = %q", got)
	}
}

func TestBuildRunArgsModel(t *testing.T) {
	args := buildRunArgs("oi", adapter.HeadlessOpts{})
	if slices.Contains(args, "--model") {
		t.Fatalf("sem Model não deve incluir --model: %v", args)
	}
	args = buildRunArgs("oi", adapter.HeadlessOpts{Model: "anthropic/claude-sonnet-4-6"})
	i := slices.Index(args, "--model")
	if i < 0 || i+1 >= len(args) || args[i+1] != "anthropic/claude-sonnet-4-6" {
		t.Fatalf("--model <model> ausente/incorreto: %v", args)
	}
}
