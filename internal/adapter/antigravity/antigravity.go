// Package antigravity implementa o Adapter para o Antigravity CLI (binário `agy`),
// que substituiu o Gemini CLI legado em 2026-05-19.
//
// Fatos confirmados na máquina (agy v1.0.10):
//   - binário "agy"; versão via `agy --version` → ex. "1.0.10".
//   - headless: `agy -p "<prompt>"` imprime TEXTO PURO (NÃO existe
//     --output-format/JSON). Flags úteis: `--print-timeout <dur>`,
//     `--model "<nome>"`.
//   - interativo + primer: `agy -i "<primer>"` (alias --prompt-interactive),
//     injeta o prompt inicial e MANTÉM a sessão aberta (mesma forma do gemini -i).
//   - auto-aprovar permissões: `--dangerously-skip-permissions`.
//   - modelos: `agy models` lista um modelo por linha (ex.: "Gemini 3.1 Pro (Low)").
//
// Injeção de MCP — DEGRADADA (sem MCP). Investigação (timebox do P1):
//   - `agy --help`/`agy help`: NÃO expõem env-override de config equivalente ao
//     antigo GEMINI_CLI_SYSTEM_SETTINGS_PATH.
//   - `strings $(which agy)`: o único env ANTIGRAVITY_/AGY_ relevante é
//     AGY_CLI_EXPERIMENTAL_RENDERING (rendering, não config). NÃO há
//     *SYSTEM_SETTINGS_PATH nem *CONFIG_PATH override.
//   - As únicas vias seriam (a) escrever `.agents/mcp_config.json` no workspace do
//     usuário ou (b) merge-e-restaura no global ~/.gemini/antigravity-cli/mcp_config.json.
//     Ambas tocam/clobberam arquivos do usuário (workspace versionado / config
//     global do CLI) — risco que o spec proíbe. Sem override seguro, o adapter
//     NÃO injeta MCP (degradação graciosa). Será revisto se surgir um override
//     documentado.
//
// Hooks/transcript também são degradados (Caps.Hooks=false, ContextMeasured=false,
// DiscoverSessions/ReadTranscript → ErrNotSupported): o histórico do agy fica em
// SQLite com payload protobuf e o formato de hook não é documentado.
package antigravity

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
)

// Adapter implementa adapter.Adapter para o Antigravity CLI.
type Adapter struct{}

// garante em tempo de compilação que *Adapter implementa as interfaces.
var (
	_ adapter.Adapter     = (*Adapter)(nil)
	_ adapter.ModelLister = (*Adapter)(nil)
)

// New cria um novo adaptador Antigravity.
func New() *Adapter { return &Adapter{} }

func (a *Adapter) ID() string { return "antigravity" }

func (a *Adapter) Capabilities() adapter.Caps {
	// Hooks=false: o formato/local de hook do agy é desconhecido — usamos
	// --dangerously-skip-permissions no interativo. Headless real via
	// `-p` (texto puro). Não aceita session-id próprio; contexto não medível
	// (histórico em SQLite/protobuf).
	return adapter.Caps{Hooks: false, Headless: true, OwnSessionID: false, ContextMeasured: false}
}

var versionRe = regexp.MustCompile(`\d+\.\d+\.\d+`)

func (a *Adapter) Detect() (adapter.Installed, error) {
	path, err := exec.LookPath("agy")
	if err != nil {
		return adapter.Installed{Present: false}, nil
	}
	ver := ""
	if out, err := exec.Command("agy", "--version").Output(); err == nil {
		ver = versionRe.FindString(string(out))
	}
	return adapter.Installed{Present: true, Path: path, Version: ver}, nil
}

func (a *Adapter) BuildInteractive(opts adapter.SpawnOpts) (adapter.CmdSpec, error) {
	args := buildInteractiveArgs(opts)
	// MCP DEGRADADO: ver comentário do pacote — sem env-override seguro do agy,
	// não injetamos MCP (não tocamos nos arquivos do usuário).
	return adapter.CmdSpec{Path: "agy", Args: args, Dir: opts.WorkingDir}, nil
}

// buildInteractiveArgs monta os args do `agy` interativo. O primer vai em
// `-i` (--prompt-interactive): injeta o prompt inicial e mantém a sessão aberta.
//
// Como Caps.Hooks=false (o worrel não consegue responder o balão de permissão do
// agy — formato de hook desconhecido), passamos --dangerously-skip-permissions
// para auto-aprovar e não travar a sessão num diálogo que ninguém responderia.
// (SpawnOpts ainda não carrega um permMode; quando carregar, condiciona-se aqui.)
func buildInteractiveArgs(opts adapter.SpawnOpts) []string {
	args := []string{}
	if strings.TrimSpace(opts.Primer) != "" {
		args = append(args, "-i", opts.Primer)
	}
	args = append(args, "--dangerously-skip-permissions")
	return args
}

// buildRunArgs monta os args do `agy` headless. Saída é texto puro (sem JSON).
// `--model` quando não-vazio; `--print-timeout` defensivo para varreduras longas.
func buildRunArgs(prompt string, opts adapter.HeadlessOpts) []string {
	args := []string{"-p", prompt}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	// --print-timeout defensivo (default do CLI é 5m; deixamos explícito p/ varreduras).
	args = append(args, "--print-timeout", "10m")
	return args
}

func (a *Adapter) RunHeadless(ctx context.Context, prompt string, opts adapter.HeadlessOpts) (string, error) {
	args := buildRunArgs(prompt, opts)
	cmd := exec.CommandContext(ctx, "agy", args...)
	cmd.Dir = opts.WorkingDir
	out, err := cmd.Output()
	if err != nil {
		return string(out), err
	}
	// Saída de `agy -p` é TEXTO PURO — sem desembrulho JSON; só trim.
	return strings.TrimSpace(string(out)), nil
}

// DiscoverSessions/ReadTranscript: histórico do agy é SQLite com payload
// protobuf — não suportado (degradação; ver P2 no spec).
func (a *Adapter) DiscoverSessions(since time.Time) ([]adapter.ExternalSession, error) {
	return nil, adapter.ErrNotSupported
}

func (a *Adapter) ReadTranscript(ref adapter.SessionRef) ([]adapter.TranscriptEvent, error) {
	return nil, adapter.ErrNotSupported
}

// ContextUsage: não medível (sem leitura do histórico).
func (a *Adapter) ContextUsage(ref adapter.SessionRef) (used, limit int, ok bool) {
	return 0, 0, false
}
