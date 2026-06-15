// Package claudecode implementa o Adapter para o CLI Claude Code.
package claudecode

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
)

// contextWindowTokens é o limite HEURÍSTICO de janela de contexto (spec §9).
// O Claude Code não expõe o limite real por modelo via CLI; 200k é a janela
// padrão dos modelos Claude atuais — suficiente para o gatilho de handoff.
const contextWindowTokens = 200000

// Adapter implementa adapter.Adapter para o Claude Code CLI.
type Adapter struct {
	// ProjectsRoot é o diretório raiz dos projetos Claude Code.
	// Vazio = ~/.claude/projects (default). Configurável para testes (fase 4).
	ProjectsRoot string
}

// New cria um novo adaptador Claude Code.
func New() *Adapter {
	root := ""
	if home, err := os.UserHomeDir(); err == nil {
		root = filepath.Join(home, ".claude", "projects")
	}
	return &Adapter{ProjectsRoot: root}
}

func (a *Adapter) ID() string { return "claude-code" }

func (a *Adapter) Capabilities() adapter.Caps {
	return adapter.Caps{Hooks: true, Headless: true, OwnSessionID: true, ContextMeasured: true}
}

var versionRe = regexp.MustCompile(`\d+\.\d+\.\d+`)

func (a *Adapter) Detect() (adapter.Installed, error) {
	path, err := exec.LookPath("claude")
	if err != nil {
		return adapter.Installed{Present: false}, nil
	}
	ver := ""
	if out, err := exec.Command("claude", "--version").Output(); err == nil {
		ver = versionRe.FindString(string(out))
	}
	return adapter.Installed{Present: true, Path: path, Version: ver}, nil
}

// mcpConfigJSON é o JSON inline passado em --mcp-config.
// Schema Claude Code: {"mcpServers": {"<nome>": {"type":"http","url":"..."}}}
func mcpConfigJSON(url string) string {
	cfg := map[string]any{
		"mcpServers": map[string]any{
			"worrel": map[string]any{"type": "http", "url": url},
		},
	}
	b, _ := json.Marshal(cfg)
	return string(b)
}

// hookMatcher escopa o PreToolUse às ferramentas que mutam/executam — sem isso,
// toda leitura de arquivo abriria um balão.
const hookMatcher = "Bash|Edit|Write|MultiEdit|NotebookEdit|WebFetch"

// writeHookSettings grava um settings temporário com o hook PreToolUse que chama
// "<selfExe> hook prompt --session <id> --port <port>" e devolve (path, cleanup).
func writeHookSettings(selfExe, sessionID string, port int) (string, func() error, error) {
	cmd := fmt.Sprintf("%s hook prompt --session %s --port %d", selfExe, sessionID, port)
	settings := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": hookMatcher,
					"hooks": []any{
						map[string]any{"type": "command", "command": cmd, "timeout": 31536000},
					},
				},
			},
		},
	}
	b, _ := json.MarshalIndent(settings, "", "  ")
	f, err := os.CreateTemp("", "worrel-settings-*.json")
	if err != nil {
		return "", nil, err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", nil, err
	}
	f.Close()
	path := f.Name()
	return path, func() error { return os.Remove(path) }, nil
}

func (a *Adapter) BuildInteractive(opts adapter.SpawnOpts) (adapter.CmdSpec, error) {
	args := []string{}
	if opts.SessionID != "" {
		args = append(args, "--session-id", opts.SessionID)
	}
	if opts.SystemAppend != "" {
		args = append(args, "--append-system-prompt", opts.SystemAppend)
	}
	if opts.MCPURL != "" {
		args = append(args, "--mcp-config", mcpConfigJSON(opts.MCPURL))
	}
	var cleanup func() error
	if opts.SelfExe != "" && opts.Port != 0 {
		path, cl, err := writeHookSettings(opts.SelfExe, opts.SessionID, opts.Port)
		if err != nil {
			return adapter.CmdSpec{}, err
		}
		args = append(args, "--settings", path)
		cleanup = cl
	}
	// primer como prompt posicional final → visível no transcript (aceitação §13.1).
	// "--" separa o primer dos flags variádicos (--mcp-config <configs...>),
	// evitando que o parser de flags consuma o primer como config adicional.
	if strings.TrimSpace(opts.Primer) != "" {
		args = append(args, "--", opts.Primer)
	}
	return adapter.CmdSpec{Path: "claude", Args: args, Dir: opts.WorkingDir, Cleanup: cleanup}, nil
}

// buildRunArgs monta os argumentos do `claude` headless. O modelo (opts.Model),
// quando não-vazio, vira `--model <model>`.
func buildRunArgs(prompt string, opts adapter.HeadlessOpts) []string {
	args := []string{"-p", prompt, "--output-format", "json"}
	if opts.MCPURL != "" {
		args = append(args, "--mcp-config", mcpConfigJSON(opts.MCPURL))
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	return args
}

func (a *Adapter) RunHeadless(ctx context.Context, prompt string, opts adapter.HeadlessOpts) (string, error) {
	cmd := exec.CommandContext(ctx, "claude", buildRunArgs(prompt, opts)...)
	cmd.Dir = opts.WorkingDir
	out, err := cmd.Output()
	if err != nil {
		return string(out), err
	}
	return extractHeadlessResult(out), nil
}

// extractHeadlessResult extrai o texto final da resposta do claude em
// --output-format json: a saída é um envelope (objeto único ou array de
// eventos) onde o item type=="result" carrega o campo "result" com o texto
// do assistente. Consumidores (varredura, handoff) precisam só do texto;
// se o envelope não for reconhecido, devolve a saída crua.
func extractHeadlessResult(out []byte) string {
	type ev struct {
		Type   string `json:"type"`
		Result string `json:"result"`
	}
	var one ev
	if err := json.Unmarshal(out, &one); err == nil && one.Type == "result" {
		return one.Result
	}
	var many []ev
	if err := json.Unmarshal(out, &many); err == nil {
		for i := len(many) - 1; i >= 0; i-- {
			if many[i].Type == "result" {
				return many[i].Result
			}
		}
	}
	return string(out)
}

// ContextUsage lê o JSONL da sessão (ref.Path, ou resolvido por glob em
// projectsRoot pelo external ref) e soma os campos de usage da ÚLTIMA
// mensagem com usage — input + cache_read + cache_creation + output — que é
// a melhor heurística do contexto ocupado. limit é contextWindowTokens.
func (a *Adapter) ContextUsage(ref adapter.SessionRef) (used, limit int, ok bool) {
	path := ref.Path
	if path == "" && ref.ExternalRef != "" && a.ProjectsRootOrDefault() != "" {
		if matches, _ := filepath.Glob(filepath.Join(a.ProjectsRootOrDefault(), "*", ref.ExternalRef+".jsonl")); len(matches) > 0 {
			path = matches[0]
		}
	}
	if path == "" {
		return 0, 0, false
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()

	type usage struct {
		InputTokens              int `json:"input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		OutputTokens             int `json:"output_tokens"`
	}
	type entry struct {
		Message *struct {
			Usage *usage `json:"usage"`
		} `json:"message"`
	}

	var last *usage
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024) // linhas do JSONL podem ser grandes
	for sc.Scan() {
		var e entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		if e.Message != nil && e.Message.Usage != nil {
			last = e.Message.Usage
		}
	}
	if last == nil {
		return 0, 0, false
	}
	used = last.InputTokens + last.CacheCreationInputTokens + last.CacheReadInputTokens + last.OutputTokens
	return used, contextWindowTokens, true
}
