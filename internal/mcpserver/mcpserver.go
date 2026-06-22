// Package mcpserver expõe a estrutura do worrel (projetos, memória, skills,
// auto-relato) como um MCP server (spec §3.3), via transporte HTTP streamable.
package mcpserver

import (
	"context"
	"log"
	"net/http"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/ask"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/vault"
)

// SummaryGenerator gera e persiste um resumo estruturado de handoff (satisfeito por *handoff.Generator).
type SummaryGenerator interface {
	GenerateSummary(ctx context.Context, sessionID string) (string, error)
}

type Service struct {
	store      *store.Store
	bus        *bus.Bus
	vault      *vault.Vault // pode ser nil se o cofre não estiver montado
	summaryGen SummaryGenerator // opcional; nil = fallback bruto
	ask        *ask.Broker // pedidos de confirmação/escolha (balões); nil = ask_user indisponível
}

func New(s *store.Store, b *bus.Bus) *Service { return &Service{store: s, bus: b} }

// NewWithVault é a variante aditiva usada quando o cofre está disponível.
func NewWithVault(s *store.Store, b *bus.Bus, v *vault.Vault) *Service {
	return &Service{store: s, bus: b, vault: v}
}

// SetVault permite anexar o cofre após a construção (merge trivial).
func (svc *Service) SetVault(v *vault.Vault) { svc.vault = v }

// WithSummaryGenerator anexa o gerador de resumo estruturado (fase 6B).
// Quando definido, get_session_summary gera-e-persiste se summary estiver vazio.
func (svc *Service) WithSummaryGenerator(g SummaryGenerator) *Service {
	svc.summaryGen = g
	return svc
}

// WithAskBroker liga o broker de balões; habilita a tool ask_user.
func (svc *Service) WithAskBroker(b *ask.Broker) *Service {
	svc.ask = b
	return svc
}

// ServerFor monta um mcp.Server com handlers atribuídos ao token de sessão
// (vazio = agente externo, sem vínculo de sessão).
func (svc *Service) ServerFor(token string) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "worrel", Version: "0.1.0"}, nil)
	a := &attribution{svc: svc, token: token}
	svc.addReadTools(srv, a)
	svc.addSessionTools(srv, a)
	svc.addSecretTools(srv, a)
	svc.addAskTools(srv, a)
	svc.addMarkAsSkillTools(srv, a)
	return srv
}

// HTTPHandler devolve o handler streamable para montar em /mcp.
func (svc *Service) HTTPHandler() http.Handler {
	return mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return svc.ServerFor(r.URL.Query().Get("s"))
	}, nil)
}

// attribution resolve sessão/projeto a partir do token (lazy, uma única vez).
type attribution struct {
	svc   *Service
	token string

	once      sync.Once
	sessionID string
	projectID string
}

// sessionProject devolve (sessionID, projectID) do token, ou ("","") se externo.
// A resolução acontece uma única vez por conexão (cache via sync.Once), então
// resolveProject + uso do sessionID custam um único lookup no store.
//
// Token inválido degrada para atribuição externa por design: as tools passam
// a exigir project_id explícito em vez de vincular silenciosamente a uma
// sessão errada. O evento fica visível no log para diagnóstico.
func (a *attribution) sessionProject() (string, string) {
	a.once.Do(func() {
		if a.token == "" {
			return
		}
		sess, err := a.svc.store.SessionByMCPToken(a.token)
		if err != nil {
			log.Printf("mcp: token inválido: %v", err)
			return
		}
		a.sessionID, a.projectID = sess.ID, sess.ProjectID
	})
	return a.sessionID, a.projectID
}

// resolveProject: usa project_id explícito; senão cai no projeto da sessão.
func (a *attribution) resolveProject(explicit string) string {
	if explicit != "" {
		return explicit
	}
	_, pid := a.sessionProject()
	return pid
}

// nilIfEmpty converte string vazia em nil (ponteiro); usada para campos opcionais.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// truncate corta em n runas (não bytes) para não quebrar UTF-8 no meio.
// Helper de pacote: usado nos títulos de sugestão e no digest de transcript.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func textResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

func errResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}
}
