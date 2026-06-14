// Package pidev implementa o Adapter para o "Pi" coding agent CLI
// (https://pi.dev, repo earendil-works/pi — pacote @earendil-works/pi-coding-agent).
//
// O que está CONFIRMADO pela documentação pública (README do pacote coding-agent
// e site pi.dev), e usado abaixo:
//   - Binário: "pi".
//   - Interativo: `pi "<prompt inicial>"` (prompt posicional opcional).
//   - Headless/print: `pi -p "<msg>"`; com saída de eventos JSON via `--mode json`.
//   - Modelo/provider: flags `--model` e `--provider`.
//   - Sessões: auto-salvas em ~/.pi/agent/sessions/, organizadas por working dir,
//     como arquivos JSONL (estrutura em árvore). Override via flag `--session-dir`
//     ou env PI_CODING_AGENT_SESSION_DIR.
//
// O que NÃO está confirmado e ficou como degradação graciosa (ErrNotSupported)
// + TODO — ver perguntas no fim do pacote:
//   - MCP: o Pi NÃO traz suporte MCP embutido; a doc sugere uma "extension".
//     Por isso BuildInteractive NÃO injeta MCPURL (sem flag/arquivo confirmado).
//   - Schema exato das linhas do JSONL de sessão (campos de role/conteúdo/usage)
//     e layout de subdiretórios por working dir → DiscoverSessions/ReadTranscript/
//     ContextUsage não implementados ainda.
package pidev

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
)

// binaryName é o nome do executável do Pi no PATH.
// CONFIRMADO pela doc oficial (curl -fsSL https://pi.dev/install.sh | sh → `pi`).
// AJUSTAR aqui caso o usuário tenha instalado sob outro nome/alias.
const binaryName = "pi"

// Adapter implementa adapter.Adapter para o Pi coding agent CLI.
type Adapter struct {
	// SessionsRoot é o diretório raiz das sessões do Pi.
	// Vazio = ~/.pi/agent/sessions (default). Configurável para testes (fase 4).
	SessionsRoot string
}

// New cria um novo adaptador Pi.
func New() *Adapter {
	root := ""
	if home, err := os.UserHomeDir(); err == nil {
		root = filepath.Join(home, ".pi", "agent", "sessions")
	}
	return &Adapter{SessionsRoot: root}
}

func (a *Adapter) ID() string { return "pidev" }

func (a *Adapter) Capabilities() adapter.Caps {
	// Conservador (spec §4 — degradação graciosa):
	//   Hooks=false        → Pi não expõe hooks de ciclo confirmados.
	//   Headless=true       → `pi -p ...` confirmado.
	//   OwnSessionID=false  → Pi gera/gerencia o id da sessão; não há flag
	//                         confirmada para forçar um UUID externo no spawn.
	//   ContextMeasured=false → schema de usage do JSONL ainda não confirmado.
	return adapter.Caps{Hooks: false, Headless: true, OwnSessionID: false, ContextMeasured: false}
}

var versionRe = regexp.MustCompile(`\d+\.\d+\.\d+`)

func (a *Adapter) Detect() (adapter.Installed, error) {
	path, err := exec.LookPath(binaryName)
	if err != nil {
		return adapter.Installed{Present: false}, nil
	}
	ver := ""
	if out, err := exec.Command(binaryName, "--version").Output(); err == nil {
		ver = versionRe.FindString(string(out))
	}
	return adapter.Installed{Present: true, Path: path, Version: ver}, nil
}

func (a *Adapter) BuildInteractive(opts adapter.SpawnOpts) (adapter.CmdSpec, error) {
	args := []string{}

	// Modelo não vem em SpawnOpts; mantido só no headless (HeadlessOpts.Model).

	// TODO(confirmar): o Pi não tem MCP embutido — a doc recomenda uma extension.
	// Quando o mecanismo for confirmado (flag de extension? arquivo de config?),
	// injetar opts.MCPURL aqui. Por ora MCPURL é ignorado conscientemente para
	// não emitir flags inexistentes que quebrariam o spawn.
	_ = opts.MCPURL
	// TODO(confirmar): não há flag confirmada para append de system prompt nem
	// para forçar SessionID externo; SystemAppend/SessionID ficam de fora.
	_ = opts.SystemAppend
	_ = opts.SessionID

	// primer como prompt posicional final → visível na sessão.
	// "--" separa o primer de quaisquer flags variádicas, por segurança.
	if strings.TrimSpace(opts.Primer) != "" {
		args = append(args, "--", opts.Primer)
	}

	spec := adapter.CmdSpec{Path: binaryName, Args: args, Dir: opts.WorkingDir}

	// Direciona as sessões para um diretório previsível quando ConfigDir é dado,
	// usando a env CONFIRMADA PI_CODING_AGENT_SESSION_DIR. Isso não cria arquivos
	// (sem Cleanup necessário); o Pi cria a sessão sozinho.
	if opts.ConfigDir != "" {
		spec.Env = append(spec.Env, "PI_CODING_AGENT_SESSION_DIR="+opts.ConfigDir)
	}
	return spec, nil
}

// buildRunArgs monta os argumentos do `pi` headless.
// CONFIRMADO: `-p <msg>` (print mode) e `--mode json` (eventos JSON).
// O modelo (opts.Model), quando não-vazio, vira `--model <model>`.
func buildRunArgs(prompt string, opts adapter.HeadlessOpts) []string {
	args := []string{"-p", prompt, "--mode", "json"}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	return args
}

func (a *Adapter) RunHeadless(ctx context.Context, prompt string, opts adapter.HeadlessOpts) (string, error) {
	cmd := exec.CommandContext(ctx, binaryName, buildRunArgs(prompt, opts)...)
	cmd.Dir = opts.WorkingDir
	out, err := cmd.Output()
	if err != nil {
		return string(out), err
	}
	// TODO(confirmar): o schema exato dos eventos de `pi --mode json` não está
	// documentado. Em vez de inventar um parser (que poderia descartar texto),
	// devolvemos a saída crua — consumidores recebem algo utilizável. Quando o
	// formato dos eventos for confirmado, extrair só o texto final do assistente
	// (vide claudecode.extractHeadlessResult / opencode.extractOpencodeText).
	return string(out), nil
}

// SessionsRootOrDefault devolve o root configurado ou o default (~/.pi/agent/sessions).
func (a *Adapter) SessionsRootOrDefault() string {
	if a.SessionsRoot != "" {
		return a.SessionsRoot
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".pi", "agent", "sessions")
	}
	return ""
}

// DiscoverSessions: degradação graciosa (spec §4). As sessões vivem em
// ~/.pi/agent/sessions/ como JSONL, mas o layout por working dir e o schema das
// linhas ainda não foram confirmados. Não suportado até confirmação.
// TODO(confirmar): layout de subdiretórios e schema do JSONL para varredura.
func (a *Adapter) DiscoverSessions(since time.Time) ([]adapter.ExternalSession, error) {
	return nil, adapter.ErrNotSupported
}

// ReadTranscript: degradação graciosa (spec §4). Depende do schema do JSONL de
// sessão, ainda não confirmado.
// TODO(confirmar): mapeamento linha JSONL → TranscriptEvent (role/kind/content/tokens).
func (a *Adapter) ReadTranscript(ref adapter.SessionRef) ([]adapter.TranscriptEvent, error) {
	return nil, adapter.ErrNotSupported
}

// ContextUsage: degradação graciosa (spec §4). Sem schema de usage confirmado no
// JSONL do Pi, não há como estimar contexto. Handoff manual continua disponível.
// TODO(confirmar): existência/nome dos campos de usage por mensagem no JSONL.
func (a *Adapter) ContextUsage(ref adapter.SessionRef) (used, limit int, ok bool) {
	return 0, 0, false
}
