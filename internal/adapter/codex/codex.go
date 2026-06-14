// Package codex implementa o Adapter para o Codex CLI da OpenAI (`codex`).
//
// Fatos confirmados (Codex CLI v0.116, openai/codex):
//   - binário: `codex`; versão via `codex --version`.
//   - interativo: `codex [PROMPT]` (TUI). Modelo via `-m`; cwd via `-C`.
//   - headless: `codex exec [PROMPT]` (alias `codex e`); `-m` modelo, `-C` cwd,
//     `-o <file>` grava a mensagem final do assistente (--output-last-message),
//     `--skip-git-repo-check` permite rodar fora de repo git, `-a never` /
//     `-s danger-full-access` para não bloquear em scans não-interativos.
//   - MCP HTTP: `[mcp_servers.<nome>] url=...` exige experimental_use_rmcp_client;
//     injetado via overrides `-c key=value` (sem mutar o config global do usuário).
//   - histórico: rollouts JSONL em ~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl
//     (ver observer.go / jsonl.go).
package codex

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
)

// contextWindowTokens é o limite HEURÍSTICO de janela de contexto (spec §9).
// Os rollouts do Codex carregam model_context_window por turno (ver
// ContextUsage), então este valor só é fallback se a janela real não aparecer.
const contextWindowTokens = 256000

// mcpServerName é o nome do servidor MCP injetado na config do Codex.
const mcpServerName = "worrel"

// Adapter implementa adapter.Adapter para o Codex CLI.
type Adapter struct {
	// SessionsRoot é o diretório raiz dos rollouts do Codex.
	// Vazio = ~/.codex/sessions (default). Configurável para testes.
	SessionsRoot string
}

// New cria um novo adaptador Codex.
func New() *Adapter {
	root := ""
	if home, err := os.UserHomeDir(); err == nil {
		root = filepath.Join(home, ".codex", "sessions")
	}
	return &Adapter{SessionsRoot: root}
}

func (a *Adapter) ID() string { return "codex" }

func (a *Adapter) Capabilities() adapter.Caps {
	// Codex TUI: sem hooks de worrel; headless via `exec`; ele gera o próprio
	// session id (rollout uuid), worrel não o injeta → OwnSessionID=false;
	// contexto medido via rollouts (token_count + model_context_window).
	return adapter.Caps{Hooks: false, Headless: true, OwnSessionID: false, ContextMeasured: true}
}

var versionRe = regexp.MustCompile(`\d+\.\d+\.\d+`)

func (a *Adapter) Detect() (adapter.Installed, error) {
	path, err := exec.LookPath("codex")
	if err != nil {
		return adapter.Installed{Present: false}, nil
	}
	ver := ""
	if out, err := exec.Command("codex", "--version").Output(); err == nil {
		ver = versionRe.FindString(string(out))
	}
	return adapter.Installed{Present: true, Path: path, Version: ver}, nil
}

// mcpConfigOverrides devolve os pares `-c key=value` que registram o servidor
// MCP HTTP do worrel sem tocar no config global. Os valores são TOML-encoded
// (strings entre aspas; bool literal), conforme o parser de `-c`.
func mcpConfigOverrides(url string) []string {
	prefix := "mcp_servers." + mcpServerName
	return []string{
		"-c", "experimental_use_rmcp_client=true",
		"-c", prefix + ".url=" + strconv.Quote(url),
	}
}

func (a *Adapter) BuildInteractive(opts adapter.SpawnOpts) (adapter.CmdSpec, error) {
	args := []string{}
	if opts.WorkingDir != "" {
		args = append(args, "-C", opts.WorkingDir)
	}
	if opts.MCPURL != "" {
		args = append(args, mcpConfigOverrides(opts.MCPURL)...)
	}
	// O primer vai como PROMPT posicional → visível na sessão (aceitação §13.1).
	// "--" separa o prompt dos flags variádicos (-c key=value ...), evitando que
	// o parser consuma o primer como override de config.
	if strings.TrimSpace(opts.Primer) != "" {
		args = append(args, "--", opts.Primer)
	}
	return adapter.CmdSpec{Path: "codex", Args: args, Dir: opts.WorkingDir}, nil
}

// buildExecArgs monta os argumentos de `codex exec`. O modelo (opts.Model),
// quando não-vazio, vira `-m <model>`. -a never / -s danger-full-access evitam
// bloqueio em execução não-interativa; --skip-git-repo-check permite cwd sem git.
func buildExecArgs(prompt string, opts adapter.HeadlessOpts, lastMsgFile string) []string {
	args := []string{"exec", "-a", "never", "--skip-git-repo-check"}
	if opts.WorkingDir != "" {
		args = append(args, "-C", opts.WorkingDir)
	}
	if opts.Model != "" {
		args = append(args, "-m", opts.Model)
	}
	if lastMsgFile != "" {
		args = append(args, "-o", lastMsgFile)
	}
	if opts.MCPURL != "" {
		args = append(args, mcpConfigOverrides(opts.MCPURL)...)
	}
	args = append(args, "--", prompt)
	return args
}

func (a *Adapter) RunHeadless(ctx context.Context, prompt string, opts adapter.HeadlessOpts) (string, error) {
	// Captura a mensagem final do assistente em arquivo via -o (mais robusto que
	// parsear o stdout formatado da TUI). Se a leitura falhar, cai no stdout cru.
	tmp, err := os.CreateTemp("", "codex-last-*.txt")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	args := buildExecArgs(prompt, opts, tmpPath)

	cmd := exec.CommandContext(ctx, "codex", args...)
	cmd.Dir = opts.WorkingDir
	out, err := cmd.Output()
	if err != nil {
		return string(out), err
	}
	if b, rerr := os.ReadFile(tmpPath); rerr == nil && strings.TrimSpace(string(b)) != "" {
		return strings.TrimSpace(string(b)), nil
	}
	return strings.TrimSpace(string(out)), nil
}
