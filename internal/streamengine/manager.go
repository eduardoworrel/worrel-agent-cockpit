package streamengine

import (
	"context"
	"sync"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/agui"
)

// Manager mantém as sessões integradas vivas, indexadas por id, e o registro de
// drivers por provider.
type Manager struct {
	mu       sync.Mutex
	sessions map[string]LiveSession
	drivers  map[string]Driver
	onChange func(sessionID string)
	persist  func(sessionID, role, text string)
}

// NewManager cria o gerenciador com os drivers padrão (DefaultDrivers).
func NewManager(onChange func(string), persist func(sessionID, role, text string)) *Manager {
	return &Manager{
		sessions: map[string]LiveSession{},
		drivers:  DefaultDrivers(),
		onChange: onChange,
		persist:  persist,
	}
}

// Start spawna e registra uma sessão do provider dado no cwd, com as opções.
func (m *Manager) Start(ctx context.Context, provider, sessionID, cwd string, o Opts) error {
	m.mu.Lock()
	drv := m.drivers[provider]
	m.mu.Unlock()
	if drv == nil {
		return ErrUnknownProvider
	}
	var persist func(role, text string)
	if m.persist != nil {
		persist = func(role, text string) { m.persist(sessionID, role, text) }
	}
	s, err := drv.Start(ctx, sessionID, cwd, o, m.onChange, persist)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.sessions[sessionID] = s
	m.mu.Unlock()
	return nil
}

// ErrUnknownProvider indica que o provider não tem driver no modo Integrado.
var ErrUnknownProvider = errUnknownProvider{}

type errUnknownProvider struct{}

func (errUnknownProvider) Error() string { return "provider não suportado no modo integrado" }

// Has diz se a sessão é dirigida pelo motor (vs. caminho legado PTY).
func (m *Manager) Has(sessionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.sessions[sessionID]
	return ok
}

func (m *Manager) get(sessionID string) LiveSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[sessionID]
}

// Snapshot devolve o Snapshot AG-UI da sessão (e se ela é do motor).
func (m *Manager) Snapshot(sessionID string) (agui.Snapshot, bool) {
	s := m.get(sessionID)
	if s == nil {
		return agui.Snapshot{}, false
	}
	return s.Snapshot(), true
}

// SendPrompt encaminha um novo turno do usuário.
func (m *Manager) SendPrompt(sessionID, text string) error {
	s := m.get(sessionID)
	if s == nil {
		return ErrNoSession
	}
	return s.SendPrompt(text)
}

// Respond responde à permissão pendente (allow/deny).
func (m *Manager) Respond(sessionID string, allow bool) error {
	s := m.get(sessionID)
	if s == nil {
		return ErrNoSession
	}
	return s.Respond(allow)
}

// Close encerra e remove a sessão.
func (m *Manager) Close(sessionID string) {
	m.mu.Lock()
	s := m.sessions[sessionID]
	delete(m.sessions, sessionID)
	m.mu.Unlock()
	if s != nil {
		s.Close()
	}
}

// ErrNoSession indica que a sessão não é dirigida pelo motor.
var ErrNoSession = errNoSession{}

type errNoSession struct{}

func (errNoSession) Error() string { return "sessão não é do motor" }
