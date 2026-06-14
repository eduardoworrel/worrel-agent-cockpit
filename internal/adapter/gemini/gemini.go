// Package gemini implementa o Adapter para o Gemini CLI (google-gemini/gemini-cli).
//
// Fatos confirmados na fonte (github.com/google-gemini/gemini-cli):
//   - binário "gemini"; versão via `gemini --version`.
//   - headless: `gemini -p "<prompt>" --output-format json`; a resposta do
//     modelo vive no campo top-level "response"; "stats.models[m].tokens" traz
//     prompt/candidates/total/cached.
//   - interativo: `gemini` no cwd; prompt inicial via `-i "<prompt>"`
//     (--prompt-interactive) que injeta o prompt e MANTÉM a sessão aberta.
//   - MCP: configurado em settings.json (mcpServers.<nome>.httpUrl p/ HTTP
//     streamable). Injetamos via env GEMINI_CLI_SYSTEM_SETTINGS_PATH apontando
//     p/ um arquivo temporário (system settings são mescladas com user/project),
//     evitando tocar no .gemini/settings.json do usuário.
//   - histórico: ~/.gemini/tmp/<id>/ por projeto, onde <id> é um slug do
//     registro ~/.gemini/projects.json (legado: sha256(projectRoot)). Cada dir
//     tem .project_root (cwd), logs.json (LogEntry[] — só mensagens do usuário),
//     chats/*.json e checkpoints/checkpoint-*.json ({history: Content[]} no
//     formato @google/genai: {role:"user"|"model", parts:[{text}]}).
package gemini

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
)

// contextWindowTokens é o limite HEURÍSTICO de janela (spec §9). Gemini 1.5/2.x
// expõem 1M tokens; o CLI não reporta o limite por modelo de forma estável, então
// usamos 1M como denominador do gatilho de handoff.
const contextWindowTokens = 1_000_000

// Adapter implementa adapter.Adapter para o Gemini CLI.
type Adapter struct {
	// TmpRoot é a raiz dos diretórios temporários por projeto do Gemini CLI.
	// Vazio = ~/.gemini/tmp (default). Configurável para testes.
	TmpRoot string
}

// garante em tempo de compilação que *Adapter implementa adapter.Adapter.
var _ adapter.Adapter = (*Adapter)(nil)

// New cria um novo adaptador Gemini.
func New() *Adapter {
	root := ""
	if home, err := os.UserHomeDir(); err == nil {
		root = filepath.Join(home, ".gemini", "tmp")
	}
	return &Adapter{TmpRoot: root}
}

func (a *Adapter) ID() string { return "gemini" }

func (a *Adapter) Capabilities() adapter.Caps {
	// Gemini CLI: sem hooks de auto-relato como o Claude Code; headless real via
	// `-p --output-format json`; NÃO aceita --session-id próprio (o id é interno);
	// contexto medível a partir de chats/checkpoints/logs no disco.
	return adapter.Caps{Hooks: false, Headless: true, OwnSessionID: false, ContextMeasured: true}
}

var versionRe = regexp.MustCompile(`\d+\.\d+\.\d+`)

func (a *Adapter) Detect() (adapter.Installed, error) {
	path, err := exec.LookPath("gemini")
	if err != nil {
		return adapter.Installed{Present: false}, nil
	}
	ver := ""
	if out, err := exec.Command("gemini", "--version").Output(); err == nil {
		ver = versionRe.FindString(string(out))
	}
	return adapter.Installed{Present: true, Path: path, Version: ver}, nil
}

// writeMCPSettings grava {"mcpServers":{"worrel":{"httpUrl":...}}} num arquivo
// dentro de configDir e devolve o caminho. É consumido via
// GEMINI_CLI_SYSTEM_SETTINGS_PATH (system settings, mescladas pelo CLI).
func writeMCPSettings(configDir, url string) (string, error) {
	if configDir == "" {
		configDir = os.TempDir()
	}
	cfg := map[string]any{
		"mcpServers": map[string]any{
			"worrel": map[string]any{"httpUrl": url},
		},
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	path := filepath.Join(configDir, "gemini-settings.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (a *Adapter) BuildInteractive(opts adapter.SpawnOpts) (adapter.CmdSpec, error) {
	args := buildInteractiveArgs(opts)
	spec := adapter.CmdSpec{Path: "gemini", Args: args, Dir: opts.WorkingDir}
	if opts.MCPURL != "" {
		path, err := writeMCPSettings(opts.ConfigDir, opts.MCPURL)
		if err != nil {
			return adapter.CmdSpec{}, err
		}
		spec.Env = append(spec.Env, "GEMINI_CLI_SYSTEM_SETTINGS_PATH="+path)
		spec.Cleanup = func() error { return os.Remove(path) }
	}
	return spec, nil
}

// buildInteractiveArgs monta os args do `gemini` interativo. O primer vai em
// `-i` (--prompt-interactive): injeta o prompt inicial e mantém a sessão aberta
// (diferente de `-p`, que é one-shot e encerra). Gemini não aceita session-id
// externo nem append-system-prompt — degradamos sem esses.
func buildInteractiveArgs(opts adapter.SpawnOpts) []string {
	args := []string{}
	if strings.TrimSpace(opts.Primer) != "" {
		args = append(args, "-i", opts.Primer)
	}
	return args
}

// buildRunArgs monta os args do `gemini` headless. O modelo (opts.Model), quando
// não-vazio, vira `-m <model>` (ex.: "gemini-2.5-pro").
func buildRunArgs(prompt string, opts adapter.HeadlessOpts) []string {
	args := []string{"-p", prompt, "--output-format", "json"}
	if opts.Model != "" {
		args = append(args, "-m", opts.Model)
	}
	return args
}

func (a *Adapter) RunHeadless(ctx context.Context, prompt string, opts adapter.HeadlessOpts) (string, error) {
	args := buildRunArgs(prompt, opts)
	cmd := exec.CommandContext(ctx, "gemini", args...)
	cmd.Dir = opts.WorkingDir
	if opts.MCPURL != "" {
		path, err := writeMCPSettings(os.TempDir(), opts.MCPURL)
		if err == nil {
			cmd.Env = append(os.Environ(), "GEMINI_CLI_SYSTEM_SETTINGS_PATH="+path)
			defer os.Remove(path)
		}
	}
	out, err := cmd.Output()
	if err != nil {
		return string(out), err
	}
	return extractHeadlessResult(out), nil
}

// extractHeadlessResult desembrulha a saída de `gemini -p --output-format json`,
// que é um único objeto com o campo top-level "response" (texto do modelo).
// Fallback: saída crua se o envelope não for reconhecido.
func extractHeadlessResult(out []byte) string {
	var env struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(out, &env); err == nil && env.Response != "" {
		return env.Response
	}
	return strings.TrimSpace(string(out))
}
