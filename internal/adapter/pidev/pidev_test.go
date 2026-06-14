package pidev

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

func TestContextUsageUnsupported(t *testing.T) {
	if _, _, ok := New().ContextUsage(adapter.SessionRef{}); ok {
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

const fixtureSessionJSONL = `{"type":"session","version":3,"id":"sess-uuid-1","timestamp":"2026-06-10T12:00:00Z","cwd":"/repos/x"}
{"type":"message","id":"m1","timestamp":"2026-06-10T12:00:01Z","message":{"role":"user","content":"como faço um build?"}}
{"type":"message","id":"m2","timestamp":"2026-06-10T12:00:02Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"hmm"},{"type":"text","text":"Use go build."}],"usage":{"input":120,"output":35,"totalTokens":155}}}
{"type":"message","id":"m3","timestamp":"2026-06-10T12:00:03Z","message":{"role":"toolResult","content":[{"type":"text","text":"ok"}],"isError":false}}
{"type":"model_change","id":"x"}
`

func writeSession(t *testing.T, root string) string {
	t.Helper()
	dir := filepath.Join(root, "--repos-x--")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "20260610_sess-uuid-1.jsonl")
	if err := os.WriteFile(p, []byte(fixtureSessionJSONL), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDiscoverSessions(t *testing.T) {
	root := t.TempDir()
	path := writeSession(t, root)
	a := &Adapter{SessionsRoot: root}

	sess, err := a.DiscoverSessions(time.Time{})
	if err != nil {
		t.Fatalf("DiscoverSessions erro: %v", err)
	}
	if len(sess) != 1 {
		t.Fatalf("quer 1 sessão, got %d", len(sess))
	}
	s := sess[0]
	if s.ExternalRef != "sess-uuid-1" {
		t.Errorf("ExternalRef = %q", s.ExternalRef)
	}
	if s.Dir != "/repos/x" {
		t.Errorf("Dir = %q", s.Dir)
	}
	if s.Title != "como faço um build?" {
		t.Errorf("Title = %q", s.Title)
	}
	if s.Path != path {
		t.Errorf("Path = %q quer %q", s.Path, path)
	}
	if s.StartedAt.IsZero() || s.UpdatedAt.IsZero() {
		t.Errorf("tempos não preenchidos: %+v", s)
	}

	// filtro por since (futuro) → nada.
	future := time.Now().Add(time.Hour)
	if got, _ := a.DiscoverSessions(future); len(got) != 0 {
		t.Errorf("since futuro deveria filtrar tudo, got %d", len(got))
	}
}

func TestReadTranscript(t *testing.T) {
	root := t.TempDir()
	path := writeSession(t, root)
	a := &Adapter{SessionsRoot: root}

	evs, err := a.ReadTranscript(adapter.SessionRef{Path: path})
	if err != nil {
		t.Fatalf("ReadTranscript erro: %v", err)
	}
	if len(evs) != 3 {
		t.Fatalf("quer 3 eventos, got %d: %+v", len(evs), evs)
	}
	if evs[0].Role != "user" || evs[0].Content != "como faço um build?" {
		t.Errorf("evento 0 errado: %+v", evs[0])
	}
	if evs[1].Role != "assistant" || evs[1].Content != "Use go build." {
		t.Errorf("evento 1 texto: %+v", evs[1])
	}
	if evs[1].TokensIn != 120 || evs[1].TokensOut != 35 {
		t.Errorf("tokens errados: %+v", evs[1])
	}
	if evs[2].Role != "toolResult" || evs[2].Content != "ok" {
		t.Errorf("evento 2 errado: %+v", evs[2])
	}
	if evs[0].CreatedAt == 0 {
		t.Error("CreatedAt não preenchido")
	}
}

func TestExtractHeadlessResult(t *testing.T) {
	out := `{"type":"session","id":"s"}
{"type":"message_start","message":{"role":"assistant"}}
{"type":"message_end","message":{"role":"assistant","content":[{"type":"thinking","thinking":"x"},{"type":"text","text":"Olá"}]}}
{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"mundo"}]}}
`
	if got := extractHeadlessResult([]byte(out)); got != "Olá\nmundo" {
		t.Fatalf("extractHeadlessResult = %q", got)
	}
	// fallback: sem eventos extraíveis → raw trim.
	raw := "texto cru\n"
	if got := extractHeadlessResult([]byte(raw)); got != "texto cru" {
		t.Fatalf("fallback = %q", got)
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
