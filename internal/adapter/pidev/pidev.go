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
// O que NÃO está confirmado e ficou como degradação graciosa:
//   - MCP: o Pi NÃO traz suporte MCP embutido; a doc sugere uma "extension".
//     Por isso BuildInteractive NÃO injeta MCPURL (sem flag/arquivo confirmado).
//   - ContextUsage: o JSONL carrega usage por mensagem assistant, mas o limite de
//     contexto (window) não é exposto de forma confiável → continua ok=false.
//
// O schema das linhas do JSONL de sessão FOI confirmado (repo earendil-works/pi):
//   - 1ª linha: {"type":"session","version":3,"id":..,"timestamp":..,"cwd":..}
//   - mensagens: {"type":"message",...,"message":{role,content,usage,...}}
// DiscoverSessions/ReadTranscript usam esse schema; RunHeadless extrai o texto
// dos eventos message_end assistant.
package pidev

import (
	"bufio"
	"context"
	"encoding/json"
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
	//   ContextMeasured=false → usage por mensagem existe, mas a janela total de
	//                            contexto não é exposta de forma confiável.
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
	return extractHeadlessResult(out), nil
}

// extractHeadlessResult percorre a saída JSONL de `pi --mode json` e concatena o
// texto dos eventos {"type":"message_end","message":{role:"assistant",content:[...]}}.
// Fallback: se nenhum texto for extraído, devolve a saída crua (trim).
func extractHeadlessResult(out []byte) string {
	var b strings.Builder
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		var ev struct {
			Type    string      `json:"type"`
			Message *piAgentMsg `json:"message"`
		}
		if json.Unmarshal(line, &ev) != nil {
			continue
		}
		if ev.Type != "message_end" || ev.Message == nil || ev.Message.Role != "assistant" {
			continue
		}
		text, _ := extractText(ev.Message.Content, ev.Message.Role)
		if text != "" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(text)
		}
	}
	if b.Len() == 0 {
		return strings.TrimSpace(string(out))
	}
	return b.String()
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

// --- Schema do JSONL de sessão do Pi (earendil-works/pi) ---

// piHeader é a 1ª linha do JSONL: {"type":"session",...}.
type piHeader struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Cwd       string `json:"cwd"`
}

// piLine é uma linha genérica do JSONL (header ou message).
type piLine struct {
	Type      string      `json:"type"`
	ID        string      `json:"id"`
	Timestamp string      `json:"timestamp"`
	Cwd       string      `json:"cwd"`
	Message   *piAgentMsg `json:"message"`
}

// piAgentMsg é o campo "message" (AgentMessage) das entradas type=="message".
type piAgentMsg struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
	Usage   *piUsage        `json:"usage"`
}

type piUsage struct {
	Input  int64 `json:"input"`
	Output int64 `json:"output"`
}

// piContentBlock é um bloco do array de content (text/thinking/etc.).
type piContentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Thinking string `json:"thinking"`
}

// extractText normaliza o content de uma mensagem do Pi em texto plano.
// user: pode ser string direta OU array de blocos {type:"text",text}.
// assistant/toolResult: array de blocos; concatena os "text".
// kind = role.
func extractText(raw json.RawMessage, role string) (text, kind string) {
	kind = role
	if len(raw) == 0 {
		return "", kind
	}
	// content como string (user).
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, kind
	}
	// content como array de blocos.
	var blocks []piContentBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return "", kind
	}
	var b strings.Builder
	for _, bl := range blocks {
		switch bl.Type {
		case "text":
			if bl.Text != "" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(bl.Text)
			}
		}
	}
	return b.String(), kind
}

// DiscoverSessions varre SessionsRoot por *.jsonl; lê a 1ª linha (header) p/
// ExternalRef(id)+Dir(cwd)+StartedAt; UpdatedAt=mtime; filtra por since (mtime);
// Title = 1º texto de user (~80 chars) ou ExternalRef. Pula arquivos sem header.
func (a *Adapter) DiscoverSessions(since time.Time) ([]adapter.ExternalSession, error) {
	root := a.SessionsRootOrDefault()
	if root == "" {
		return nil, nil
	}
	var out []adapter.ExternalSession
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if !since.IsZero() && info.ModTime().Before(since) {
			return nil
		}
		es, ok := a.scanSessionMeta(path, info.ModTime())
		if ok {
			out = append(out, es)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// scanSessionMeta lê o header (1ª linha) e o 1º texto de user para metadados.
func (a *Adapter) scanSessionMeta(path string, mtime time.Time) (adapter.ExternalSession, bool) {
	f, err := os.Open(path)
	if err != nil {
		return adapter.ExternalSession{}, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	if !sc.Scan() {
		return adapter.ExternalSession{}, false
	}
	var hdr piHeader
	if json.Unmarshal(sc.Bytes(), &hdr) != nil || hdr.Type != "session" || hdr.ID == "" {
		return adapter.ExternalSession{}, false
	}
	es := adapter.ExternalSession{
		Adapter:     a.ID(),
		ExternalRef: hdr.ID,
		Dir:         hdr.Cwd,
		Path:        path,
		UpdatedAt:   mtime,
	}
	if t, err := time.Parse(time.RFC3339, hdr.Timestamp); err == nil {
		es.StartedAt = t
	}
	// procura o 1º texto de user para o título.
	for sc.Scan() {
		var ln piLine
		if json.Unmarshal(sc.Bytes(), &ln) != nil {
			continue
		}
		if ln.Type != "message" || ln.Message == nil || ln.Message.Role != "user" {
			continue
		}
		text, _ := extractText(ln.Message.Content, ln.Message.Role)
		text = strings.TrimSpace(text)
		if text != "" {
			es.Title = truncate(text, 80)
			break
		}
	}
	if es.Title == "" {
		es.Title = es.ExternalRef
	}
	return es, true
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// ReadTranscript abre ref.Path e converte cada linha type=="message" (user/
// assistant/toolResult) em TranscriptEvent. Pula entradas vazias.
func (a *Adapter) ReadTranscript(ref adapter.SessionRef) ([]adapter.TranscriptEvent, error) {
	f, err := os.Open(ref.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []adapter.TranscriptEvent
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		var ln piLine
		if json.Unmarshal(sc.Bytes(), &ln) != nil {
			continue
		}
		if ln.Type != "message" || ln.Message == nil {
			continue
		}
		switch ln.Message.Role {
		case "user", "assistant", "toolResult":
		default:
			continue
		}
		text, kind := extractText(ln.Message.Content, ln.Message.Role)
		if strings.TrimSpace(text) == "" {
			continue
		}
		te := adapter.TranscriptEvent{Role: ln.Message.Role, Kind: kind, Content: text}
		if ln.Message.Usage != nil {
			te.TokensIn = ln.Message.Usage.Input
			te.TokensOut = ln.Message.Usage.Output
		}
		if t, err := time.Parse(time.RFC3339, ln.Timestamp); err == nil {
			te.CreatedAt = t.UnixMilli()
		}
		out = append(out, te)
	}
	return out, sc.Err()
}

// ContextUsage: degradação graciosa. O JSONL carrega usage por mensagem, mas a
// janela total de contexto não é exposta de forma confiável → ok=false.
func (a *Adapter) ContextUsage(ref adapter.SessionRef) (used, limit int, ok bool) {
	return 0, 0, false
}
