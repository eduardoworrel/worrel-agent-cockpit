package antigravity

import (
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
)

func TestIDAndCaps(t *testing.T) {
	a := New()
	if a.ID() != "antigravity" {
		t.Fatalf("ID = %q, want antigravity", a.ID())
	}
	c := a.Capabilities()
	if c.Hooks {
		t.Errorf("Hooks should be false (formato de hook do agy desconhecido)")
	}
	if !c.Headless {
		t.Errorf("Headless should be true")
	}
	if c.OwnSessionID {
		t.Errorf("OwnSessionID should be false")
	}
	if c.ContextMeasured {
		t.Errorf("ContextMeasured should be false")
	}
}

func TestVersionRe(t *testing.T) {
	cases := map[string]string{
		"1.0.10":            "1.0.10",
		"agy version 1.2.3": "1.2.3",
		"v12.34.56 (build)": "12.34.56",
		"no version here":   "",
	}
	for in, want := range cases {
		if got := versionRe.FindString(in); got != want {
			t.Errorf("versionRe(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDetectAbsent(t *testing.T) {
	// Não dá para garantir ausência do binário no ambiente; validamos só que
	// Detect nunca devolve erro (degradação graciosa) e é consistente com LookPath.
	inst, err := New().Detect()
	if err != nil {
		t.Fatalf("Detect não deveria errar: %v", err)
	}
	if inst.Present && inst.Path == "" {
		t.Errorf("Present=true exige Path não-vazio: %+v", inst)
	}
}

func TestBuildInteractiveArgs(t *testing.T) {
	// Sem primer: só o skip-permissions.
	got := buildInteractiveArgs(adapter.SpawnOpts{})
	want := []string{"--dangerously-skip-permissions"}
	if !equal(got, want) {
		t.Errorf("buildInteractiveArgs(empty) = %v, want %v", got, want)
	}
	// Com primer.
	got = buildInteractiveArgs(adapter.SpawnOpts{Primer: "start here"})
	want = []string{"-i", "start here", "--dangerously-skip-permissions"}
	if !equal(got, want) {
		t.Errorf("buildInteractiveArgs = %v, want %v", got, want)
	}
	// Primer só com espaços é ignorado.
	got = buildInteractiveArgs(adapter.SpawnOpts{Primer: "   "})
	want = []string{"--dangerously-skip-permissions"}
	if !equal(got, want) {
		t.Errorf("blank primer = %v, want %v", got, want)
	}
}

func TestBuildInteractiveSpec(t *testing.T) {
	spec, err := New().BuildInteractive(adapter.SpawnOpts{WorkingDir: "/work", Primer: "go"})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Path != "agy" || spec.Dir != "/work" {
		t.Errorf("spec inesperado: %+v", spec)
	}
	if !contains(spec.Args, "-i") || !contains(spec.Args, "go") {
		t.Errorf("primer fora dos args: %v", spec.Args)
	}
	// MCP degradado: não deve haver env nem cleanup.
	if len(spec.Env) != 0 {
		t.Errorf("MCP degradado não deveria setar Env: %v", spec.Env)
	}
	if spec.Cleanup != nil {
		t.Errorf("MCP degradado não deveria ter Cleanup")
	}
}

func TestBuildRunArgs(t *testing.T) {
	args := buildRunArgs("hello world", adapter.HeadlessOpts{})
	want := []string{"-p", "hello world", "--print-timeout", "10m"}
	if !equal(args, want) {
		t.Errorf("buildRunArgs = %v, want %v", args, want)
	}
	args = buildRunArgs("hi", adapter.HeadlessOpts{Model: "Gemini 3.1 Pro (Low)"})
	want = []string{"-p", "hi", "--model", "Gemini 3.1 Pro (Low)", "--print-timeout", "10m"}
	if !equal(args, want) {
		t.Errorf("buildRunArgs+model = %v, want %v", args, want)
	}
}

func TestParseModels(t *testing.T) {
	out := []byte("Gemini 3.1 Pro (Low)\n  Claude 4.6  \n\nGPT-OSS\n")
	got := parseModels(out)
	want := []string{"Gemini 3.1 Pro (Low)", "Claude 4.6", "GPT-OSS"}
	if !equal(got, want) {
		t.Errorf("parseModels = %v, want %v", got, want)
	}
	if got := parseModels([]byte("   \n\n")); len(got) != 0 {
		t.Errorf("parseModels(vazio) = %v, want []", got)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
