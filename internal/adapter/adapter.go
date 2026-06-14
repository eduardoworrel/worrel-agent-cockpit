// Package adapter normaliza "como spawnar e operar" cada CLI agêntico
// (Claude Code, OpenCode), conforme a interface da spec (§"Interface de adaptador").
//
// Na fase 3 só ID/Detect/Capabilities/BuildInteractive/RunHeadless são reais;
// DiscoverSessions/ReadTranscript devolvem ErrNotSupported (fase 4 os preenche).
package adapter

import (
	"context"
	"errors"
	"time"
)

// ErrNotSupported indica capacidade ainda não implementada neste adaptador.
var ErrNotSupported = errors.New("adapter: não suportado")

// Installed descreve a presença do CLI no PATH.
type Installed struct {
	Present bool   `json:"present"`
	Path    string `json:"path"`
	Version string `json:"version"`
}

// Caps lista o que o adaptador consegue fazer (degradação graciosa — spec §4).
type Caps struct {
	Hooks           bool `json:"hooks"`
	Headless        bool `json:"headless"`
	OwnSessionID    bool `json:"own_session_id"`
	ContextMeasured bool `json:"context_measured"`
}

// SpawnOpts é tudo que o wrapper precisa para montar a sessão interativa.
type SpawnOpts struct {
	SessionID    string   // UUID gerado pelo worrel para esta sessão
	WorkingDir   string   // cwd do projeto (primeiro dir do projeto, ou "")
	Primer       string   // memória do projeto + instruções; vai como prompt inicial visível
	SystemAppend string   // instruções de auto-relato (system prompt)
	MCPURL       string   // http://127.0.0.1:<port>/mcp?s=<token>
	ConfigDir    string   // diretório temporário escrevível p/ arquivos de config do CLI
	ExtraEnv     []string // env vars adicionais (formato KEY=VALUE); fase 5 injeta vault.InjectableEnv aqui sem re-editar wrapper.go
}

// CmdSpec é o resultado puro de BuildInteractive: o que o PTY vai executar.
type CmdSpec struct {
	Path string   // binário (ex.: "claude")
	Args []string // argumentos (sem o Path)
	Env  []string // env extra (formato KEY=VALUE), somado ao os.Environ()
	Dir  string   // cwd
	// Cleanup remove arquivos temporários criados por BuildInteractive (config files).
	// Pode ser nil.
	Cleanup func() error `json:"-"`
}

// HeadlessOpts para varreduras/handoff (fase 4+).
type HeadlessOpts struct {
	WorkingDir string
	MCPURL     string
	// Model sobrescreve o modelo do CLI para esta execução (vazio = default do CLI).
	// Formato depende do adapter: opencode usa "provider/model"
	// (ex.: "anthropic/claude-sonnet-4-6"); claude-code usa o id (ex.: "claude-sonnet-4-6").
	Model string
}

// SessionRef referencia uma sessão externa (fase 4).
type SessionRef struct {
	Adapter     string
	ExternalRef string
	Path        string // jsonl path (claude) — opcional p/ opencode
}

// ExternalSession e TranscriptEvent: formatos normalizados (fase 4).
type ExternalSession struct {
	Adapter     string
	ExternalRef string
	Title       string
	Dir         string
	// Path é o caminho do arquivo de transcript quando aplicável (ex.: .jsonl para claude-code).
	// Vazio para adapters que não usam arquivo (ex.: opencode usa ExternalRef via DB).
	Path      string
	StartedAt time.Time
	UpdatedAt time.Time
}

// TranscriptEvent é um evento normalizado de transcript (fase 4).
type TranscriptEvent struct {
	Role      string
	Kind      string
	Content   string
	TokensIn  int64
	TokensOut int64
	CreatedAt int64 // unix ms
}

// ModelLister é uma capacidade OPCIONAL: adaptadores que conseguem enumerar os
// modelos disponíveis para a subscription/login ativo do usuário a implementam.
// Não faz parte da interface Adapter obrigatória — o consumidor (httpapi) detecta
// via type assertion (a, ok := adapter.(ModelLister)). Quando o CLI expõe um
// comando real de listagem, ListModels deve preferi-lo (reflete a subscription);
// só cai em lista curada quando o CLI não oferece o comando. Se o CLI não estiver
// instalado, deve degradar retornando ([]string{}, erro).
type ModelLister interface {
	// ListModels devolve os ids de modelo disponíveis (formato depende do CLI,
	// ex.: opencode usa "provider/model"; claude-code usa o id direto).
	ListModels(ctx context.Context) ([]string, error)
}

// Adapter é a interface implementada por cada CLI suportado.
type Adapter interface {
	ID() string
	Detect() (Installed, error)
	Capabilities() Caps
	BuildInteractive(opts SpawnOpts) (CmdSpec, error)
	RunHeadless(ctx context.Context, prompt string, opts HeadlessOpts) (string, error)
	DiscoverSessions(since time.Time) ([]ExternalSession, error)
	ReadTranscript(ref SessionRef) ([]TranscriptEvent, error)
	ContextUsage(ref SessionRef) (used, limit int, ok bool)
}
