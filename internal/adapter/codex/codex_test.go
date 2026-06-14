package codex

import (
	"strings"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
)

func TestID(t *testing.T) {
	if got := New().ID(); got != "codex" {
		t.Fatalf("ID() = %q, quer %q", got, "codex")
	}
}

func TestCapabilities(t *testing.T) {
	c := New().Capabilities()
	if c.Hooks {
		t.Error("Hooks deveria ser false")
	}
	if !c.Headless {
		t.Error("Headless deveria ser true")
	}
	if c.OwnSessionID {
		t.Error("OwnSessionID deveria ser false")
	}
	if !c.ContextMeasured {
		t.Error("ContextMeasured deveria ser true")
	}
}

func TestVersionRegex(t *testing.T) {
	cases := map[string]string{
		"codex-cli 0.116.0":     "0.116.0",
		"codex 1.2.3 (release)": "1.2.3",
		"0.0.1\n":               "0.0.1",
		"sem versao":            "",
	}
	for in, want := range cases {
		if got := versionRe.FindString(in); got != want {
			t.Errorf("versionRe(%q) = %q, quer %q", in, got, want)
		}
	}
}

func TestBuildInteractive(t *testing.T) {
	a := New()
	spec, err := a.BuildInteractive(adapter.SpawnOpts{
		WorkingDir: "/tmp/proj",
		Primer:     "olá agente",
		MCPURL:     "http://127.0.0.1:9000/mcp?s=tok",
	})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Path != "codex" {
		t.Fatalf("Path = %q", spec.Path)
	}
	if spec.Dir != "/tmp/proj" {
		t.Fatalf("Dir = %q", spec.Dir)
	}
	args := strings.Join(spec.Args, " ")
	if !strings.Contains(args, "-C /tmp/proj") {
		t.Errorf("faltou cwd: %v", spec.Args)
	}
	if !strings.Contains(args, "experimental_use_rmcp_client=true") {
		t.Errorf("faltou flag rmcp: %v", spec.Args)
	}
	if !strings.Contains(args, `mcp_servers.worrel.url="http://127.0.0.1:9000/mcp?s=tok"`) {
		t.Errorf("faltou url MCP: %v", spec.Args)
	}
	// primer deve vir após "--"
	last := spec.Args[len(spec.Args)-2:]
	if last[0] != "--" || last[1] != "olá agente" {
		t.Errorf("primer mal posicionado: %v", spec.Args)
	}
}

func TestBuildInteractiveMinimal(t *testing.T) {
	spec, err := New().BuildInteractive(adapter.SpawnOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Args) != 0 {
		t.Errorf("sem opts, args deveria ser vazio: %v", spec.Args)
	}
}

func TestBuildExecArgs(t *testing.T) {
	args := buildExecArgs("faça X", adapter.HeadlessOpts{
		WorkingDir: "/w",
		Model:      "gpt-5.4",
		MCPURL:     "http://h/mcp",
	}, "/tmp/last.txt")
	joined := strings.Join(args, " ")
	if args[0] != "exec" {
		t.Fatalf("primeiro arg = %q, quer exec", args[0])
	}
	for _, want := range []string{"-a never", "--skip-git-repo-check", "-C /w", "-m gpt-5.4", "-o /tmp/last.txt"} {
		if !strings.Contains(joined, want) {
			t.Errorf("faltou %q em %v", want, args)
		}
	}
	if args[len(args)-2] != "--" || args[len(args)-1] != "faça X" {
		t.Errorf("prompt mal posicionado: %v", args)
	}
}

func TestBuildExecArgsMinimal(t *testing.T) {
	args := buildExecArgs("p", adapter.HeadlessOpts{}, "")
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "-m ") || strings.Contains(joined, "-C ") || strings.Contains(joined, "-o ") {
		t.Errorf("flags opcionais não deveriam aparecer: %v", args)
	}
}

func TestDetectAbsent(t *testing.T) {
	// Não assume codex instalado; só garante que Detect não entra em pânico e
	// devolve coerência entre Present e Path.
	inst, err := New().Detect()
	if err != nil {
		t.Fatal(err)
	}
	if inst.Present && inst.Path == "" {
		t.Error("Present=true mas Path vazio")
	}
}
