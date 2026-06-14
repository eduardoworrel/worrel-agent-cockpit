// Package opencode implementa o Adapter para o CLI OpenCode.
package opencode

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

// Adapter implementa adapter.Adapter para o OpenCode CLI.
type Adapter struct {
	// DBPath é o caminho do banco de dados do opencode.
	// Vazio = ~/.local/share/opencode/opencode.db (default). Configurável para testes (fase 4).
	DBPath string
}

// New cria um novo adaptador OpenCode.
func New() *Adapter { return &Adapter{} }

func (a *Adapter) ID() string { return "opencode" }

func (a *Adapter) Capabilities() adapter.Caps {
	// OpenCode TUI: sem hooks; headless via `run`; sessão própria; contexto via DB (fase 4).
	return adapter.Caps{Hooks: false, Headless: true, OwnSessionID: false, ContextMeasured: true}
}

var versionRe = regexp.MustCompile(`\d+\.\d+\.\d+`)

func (a *Adapter) Detect() (adapter.Installed, error) {
	path, err := exec.LookPath("opencode")
	if err != nil {
		return adapter.Installed{Present: false}, nil
	}
	ver := ""
	if out, err := exec.Command("opencode", "--version").Output(); err == nil {
		ver = versionRe.FindString(string(out))
	}
	return adapter.Installed{Present: true, Path: path, Version: ver}, nil
}

// writeMCPConfig grava {"mcp":{"worrel":{"type":"remote","url":...,"enabled":true}}}
// num arquivo dentro de configDir e devolve o caminho.
func writeMCPConfig(configDir, url string) (string, error) {
	if configDir == "" {
		configDir = os.TempDir()
	}
	cfg := map[string]any{
		"mcp": map[string]any{
			"worrel": map[string]any{"type": "remote", "url": url, "enabled": true},
		},
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	path := filepath.Join(configDir, "opencode.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (a *Adapter) BuildInteractive(opts adapter.SpawnOpts) (adapter.CmdSpec, error) {
	args := []string{}
	if opts.WorkingDir != "" {
		args = append(args, opts.WorkingDir) // path posicional do projeto
	}
	if strings.TrimSpace(opts.Primer) != "" {
		args = append(args, "--prompt", opts.Primer)
	}
	spec := adapter.CmdSpec{Path: "opencode", Args: args, Dir: opts.WorkingDir}
	if opts.MCPURL != "" {
		path, err := writeMCPConfig(opts.ConfigDir, opts.MCPURL)
		if err != nil {
			return adapter.CmdSpec{}, err
		}
		spec.Env = append(spec.Env, "OPENCODE_CONFIG="+path)
		spec.Cleanup = func() error { return os.Remove(path) }
	}
	return spec, nil
}

// buildRunArgs monta os argumentos de `opencode run`. O modelo (opts.Model),
// quando não-vazio, vira `--model <model>` (formato "provider/model").
func buildRunArgs(prompt string, opts adapter.HeadlessOpts) []string {
	args := []string{"run", prompt, "--format", "json"}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	return args
}

func (a *Adapter) RunHeadless(ctx context.Context, prompt string, opts adapter.HeadlessOpts) (string, error) {
	cmd := exec.CommandContext(ctx, "opencode", buildRunArgs(prompt, opts)...)
	cmd.Dir = opts.WorkingDir
	out, err := cmd.Output()
	if err != nil {
		return string(out), err
	}
	return extractOpencodeText(out), nil
}

// extractOpencodeText desembrulha a saída de `opencode run --format json`, que é
// um stream NDJSON de eventos. O texto do modelo vive nos eventos type=="text"
// (part.text); concatenamos esses para devolver só a resposta do modelo — que é
// o que os parsers de varredura/clusterização esperam. Sem isso, o JSON do
// envelope poluiria o parse. Fallback: saída crua se nada for reconhecido.
func extractOpencodeText(out []byte) string {
	var b strings.Builder
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev struct {
			Type string `json:"type"`
			Part struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"part"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if ev.Type == "text" || ev.Part.Type == "text" {
			b.WriteString(ev.Part.Text)
		}
	}
	if b.Len() == 0 {
		return string(out)
	}
	return b.String()
}

// ContextUsage: degradação graciosa (spec §4). O observer (fase 4) já lê o
// sqlite do OpenCode; somar tokens_* de lá para estimar contexto fica como
// melhoria futura — por ora ok=false (handoff manual continua disponível).
func (a *Adapter) ContextUsage(ref adapter.SessionRef) (used, limit int, ok bool) { return 0, 0, false }
