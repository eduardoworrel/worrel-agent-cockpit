package gemini

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
)

func TestIDAndCaps(t *testing.T) {
	a := New()
	if a.ID() != "gemini" {
		t.Fatalf("ID = %q, want gemini", a.ID())
	}
	c := a.Capabilities()
	if c.Hooks {
		t.Errorf("Hooks should be false")
	}
	if !c.Headless {
		t.Errorf("Headless should be true")
	}
	if c.OwnSessionID {
		t.Errorf("OwnSessionID should be false")
	}
	if !c.ContextMeasured {
		t.Errorf("ContextMeasured should be true")
	}
}

func TestVersionRe(t *testing.T) {
	cases := map[string]string{
		"0.1.5":                "0.1.5",
		"gemini version 1.2.3": "1.2.3",
		"v12.34.56 (build)":    "12.34.56",
		"no version here":      "",
	}
	for in, want := range cases {
		if got := versionRe.FindString(in); got != want {
			t.Errorf("versionRe(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildRunArgs(t *testing.T) {
	args := buildRunArgs("hello world", adapter.HeadlessOpts{})
	want := []string{"-p", "hello world", "--output-format", "json"}
	if !equal(args, want) {
		t.Errorf("buildRunArgs = %v, want %v", args, want)
	}

	args = buildRunArgs("hi", adapter.HeadlessOpts{Model: "gemini-2.5-pro"})
	want = []string{"-p", "hi", "--output-format", "json", "-m", "gemini-2.5-pro"}
	if !equal(args, want) {
		t.Errorf("buildRunArgs+model = %v, want %v", args, want)
	}
}

func TestBuildInteractiveArgs(t *testing.T) {
	if got := buildInteractiveArgs(adapter.SpawnOpts{}); len(got) != 0 {
		t.Errorf("empty opts should yield no args, got %v", got)
	}
	got := buildInteractiveArgs(adapter.SpawnOpts{Primer: "start here"})
	want := []string{"-i", "start here"}
	if !equal(got, want) {
		t.Errorf("buildInteractiveArgs = %v, want %v", got, want)
	}
	// whitespace-only primer is ignored
	if got := buildInteractiveArgs(adapter.SpawnOpts{Primer: "   "}); len(got) != 0 {
		t.Errorf("blank primer should yield no args, got %v", got)
	}
}

func TestBuildInteractiveMCP(t *testing.T) {
	dir := t.TempDir()
	a := New()
	spec, err := a.BuildInteractive(adapter.SpawnOpts{
		WorkingDir: "/work",
		Primer:     "go",
		MCPURL:     "http://127.0.0.1:9000/mcp?s=tok",
		ConfigDir:  dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Path != "gemini" || spec.Dir != "/work" {
		t.Errorf("unexpected spec: %+v", spec)
	}
	if !contains(spec.Args, "-i") || !contains(spec.Args, "go") {
		t.Errorf("primer not in args: %v", spec.Args)
	}
	settingsPath := filepath.Join(dir, "gemini-settings.json")
	foundEnv := false
	for _, e := range spec.Env {
		if e == "GEMINI_CLI_SYSTEM_SETTINGS_PATH="+settingsPath {
			foundEnv = true
		}
	}
	if !foundEnv {
		t.Errorf("settings env not set: %v", spec.Env)
	}
	b, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings file missing: %v", err)
	}
	if !containsSub(string(b), "httpUrl") {
		t.Errorf("settings missing httpUrl: %s", b)
	}
	if !containsSub(string(b), "http://127.0.0.1:9000/mcp?s=tok") {
		t.Errorf("settings missing url: %s", b)
	}
	if spec.Cleanup != nil {
		if err := spec.Cleanup(); err != nil {
			t.Errorf("cleanup: %v", err)
		}
		if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
			t.Errorf("cleanup did not remove settings file")
		}
	}
}

func TestExtractHeadlessResult(t *testing.T) {
	out := []byte(`{"response":"the answer is 42","stats":{"models":{}}}`)
	if got := extractHeadlessResult(out); got != "the answer is 42" {
		t.Errorf("got %q", got)
	}
	// fallback to raw when no response field
	raw := []byte(`plain text output`)
	if got := extractHeadlessResult(raw); got != "plain text output" {
		t.Errorf("fallback got %q", got)
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

func containsSub(s, sub string) bool { return strings.Contains(s, sub) }
