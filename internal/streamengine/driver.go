// internal/streamengine/driver.go
package streamengine

import (
	"context"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/agui"
)

// LiveSession é uma sessão integrada viva, agnóstica de provider. O Manager fala
// só com esta interface; cada provider (Claude, OpenCode, …) fornece sua impl.
type LiveSession interface {
	Snapshot() agui.Snapshot
	SendPrompt(text string) error
	Respond(allow bool) error
	Close()
}

// Driver spawna uma LiveSession para um provider. onChange é chamado a cada
// transição de estado; persist grava cada linha do histórico (pode ser nil).
type Driver interface {
	Start(ctx context.Context, sessionID, cwd string, o Opts,
		onChange func(string), persist func(role, text string)) (LiveSession, error)
}

// claudeDriver é a impl de referência: o motor stream-json nativo do Claude Code.
type claudeDriver struct{}

func (claudeDriver) Start(ctx context.Context, sessionID, cwd string, o Opts,
	onChange func(string), persist func(role, text string)) (LiveSession, error) {
	return Start(ctx, sessionID, cwd, o, onChange, persist)
}

// DefaultDrivers é o registro de providers suportados no modo Integrado.
// agy/antigravity NÃO aparece aqui de propósito: não tem protocolo stream.
func DefaultDrivers() map[string]Driver {
	return map[string]Driver{
		"claude-code": claudeDriver{},
		"opencode":    opencodeDriver{},
		"codex":       codexDriver{},
	}
}
