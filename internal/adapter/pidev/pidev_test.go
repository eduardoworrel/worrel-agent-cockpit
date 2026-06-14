package pidev

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
)

func TestNewAndID(t *testing.T) {
	a := New()
	if a == nil {
		t.Fatal("New() devolveu nil")
	}
	if got := a.ID(); got != "pidev" {
		t.Fatalf("ID() = %q, quer %q", got, "pidev")
	}
}

// Garante que o tipo satisfaz a interface em tempo de compilação.
var _ adapter.Adapter = (*Adapter)(nil)

func TestDetectNoPanic(t *testing.T) {
	// Detect não deve entrar em panic, independentemente de o "pi" estar no PATH.
	inst, err := New().Detect()
	if err != nil {
		t.Fatalf("Detect() erro inesperado: %v", err)
	}
	if inst.Present && inst.Path == "" {
		t.Fatal("Detect() marcou Present mas sem Path")
	}
}

func TestCapabilitiesConservative(t *testing.T) {
	c := New().Capabilities()
	if !c.Headless {
		t.Error("esperado Headless=true (pi -p é confirmado)")
	}
	if c.Hooks || c.OwnSessionID || c.ContextMeasured {
		t.Errorf("capabilities deveriam ser conservadoras, got %+v", c)
	}
}

func TestBuildInteractive(t *testing.T) {
	a := New()
	spec, err := a.BuildInteractive(adapter.SpawnOpts{
		Primer:     "olá mundo",
		WorkingDir: "/tmp/projeto",
		ConfigDir:  "/tmp/cfg",
		MCPURL:     "http://127.0.0.1:9/mcp",
	})
	if err != nil {
		t.Fatalf("BuildInteractive erro: %v", err)
	}
	if spec.Path != binaryName {
		t.Fatalf("Path = %q, quer %q", spec.Path, binaryName)
	}
	if spec.Dir != "/tmp/projeto" {
		t.Fatalf("Dir = %q", spec.Dir)
	}
	if !slices.Contains(spec.Args, "olá mundo") {
		t.Fatalf("primer ausente nos args: %v", spec.Args)
	}
	if !slices.Contains(spec.Env, "PI_CODING_AGENT_SESSION_DIR=/tmp/cfg") {
		t.Fatalf("env de sessão ausente: %v", spec.Env)
	}
}

func TestBuildRunArgs(t *testing.T) {
	args := buildRunArgs("pergunta", adapter.HeadlessOpts{Model: "anthropic/claude"})
	if !slices.Contains(args, "-p") || !slices.Contains(args, "pergunta") {
		t.Fatalf("args de print mode ausentes: %v", args)
	}
	if !slices.Contains(args, "--model") || !slices.Contains(args, "anthropic/claude") {
		t.Fatalf("flag de modelo ausente: %v", args)
	}
}

func TestUnsupportedMethodsReturnErrNotSupported(t *testing.T) {
	a := New()

	if _, err := a.DiscoverSessions(time.Now()); !errors.Is(err, adapter.ErrNotSupported) {
		t.Errorf("DiscoverSessions: quer ErrNotSupported, got %v", err)
	}
	if _, err := a.ReadTranscript(adapter.SessionRef{}); !errors.Is(err, adapter.ErrNotSupported) {
		t.Errorf("ReadTranscript: quer ErrNotSupported, got %v", err)
	}
	if _, _, ok := a.ContextUsage(adapter.SessionRef{}); ok {
		t.Error("ContextUsage: quer ok=false")
	}
}

func TestRunHeadlessFailsGracefullyWhenBinaryMissing(t *testing.T) {
	// Não deve entrar em panic mesmo sem o binário; só checamos que retorna.
	if _, err := exec_lookable(); err == nil {
		// pi instalado: pulamos para não disparar uma execução real.
		t.Skip("pi presente no PATH; pulando execução real")
	}
	_, err := New().RunHeadless(context.Background(), "oi", adapter.HeadlessOpts{})
	if err == nil {
		t.Error("esperado erro quando o binário não existe")
	}
}

// exec_lookable reporta se o binário do Pi está no PATH (via Detect).
func exec_lookable() (bool, error) {
	inst, err := New().Detect()
	if err != nil {
		return false, err
	}
	if inst.Present {
		return true, nil
	}
	return false, errors.New("ausente")
}
