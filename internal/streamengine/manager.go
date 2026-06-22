package streamengine

import (
	"context"
	"sync"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/agui"
)

// Manager mantém as sessões dirigidas pelo motor stream-json, indexadas por id.
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session
	onChange func(sessionID string)
	// persist grava cada linha do histórico de uma sessão no store (durável). É
	// o que faz o chat sobreviver ao restart do app. Pode ser nil.
	persist func(sessionID, role, text string)
}

// NewManager cria o gerenciador. onChange é chamado a cada transição de qualquer
// sessão (a borda HTTP publica isso no bus para a Home rebuscar). persist grava
// cada linha do histórico no store para o chat sobreviver ao restart (pode ser nil).
func NewManager(onChange func(string), persist func(sessionID, role, text string)) *Manager {
	return &Manager{sessions: map[string]*Session{}, onChange: onChange, persist: persist}
}

// Start spawna e registra uma sessão do motor no cwd dado, com as opções.
func (m *Manager) Start(ctx context.Context, sessionID, cwd string, o Opts) error {
	var persist func(role, text string)
	if m.persist != nil {
		persist = func(role, text string) { m.persist(sessionID, role, text) }
	}
	s, err := Start(ctx, sessionID, cwd, o, m.onChange, persist)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.sessions[sessionID] = s
	m.mu.Unlock()
	return nil
}

// Has diz se a sessão é dirigida pelo motor (vs. caminho legado PTY).
func (m *Manager) Has(sessionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.sessions[sessionID]
	return ok
}

func (m *Manager) get(sessionID string) *Session {
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
