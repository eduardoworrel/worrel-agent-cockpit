package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/ask"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
)

type askUserArgs struct {
	Question string   `json:"question" jsonschema:"a pergunta ou confirmação a fazer ao usuário"`
	Options  []string `json:"options,omitempty" jsonschema:"opções clicáveis; vazio = resposta de texto livre"`
}

func (svc *Service) addAskTools(srv *mcp.Server, a *attribution) {
	if svc.ask == nil {
		return
	}
	mcp.AddTool(srv, &mcp.Tool{Name: "ask_user",
		Description: "Pergunta algo ao usuário e BLOQUEIA até ele responder na interface (um balão). " +
			"Use para confirmar uma ação ou pedir uma escolha em vez de assumir. " +
			"Passe 'options' para escolhas clicáveis; deixe vazio para uma resposta de texto livre."},
		func(ctx context.Context, req *mcp.CallToolRequest, in askUserArgs) (*mcp.CallToolResult, any, error) {
			sid, pid := a.sessionProject()
			if sid == "" {
				return errResult("ask_user requer uma sessão vinculada"), nil, nil
			}
			return svc.handleAskUser(ctx, sid, pid, in), nil, nil
		})
}

// handleAskUser é a lógica testável de ask_user: abre o pedido no broker, publica
// ask.requested, bloqueia até resposta (ou cancelamento), e devolve o texto.
func (svc *Service) handleAskUser(ctx context.Context, sessionID, projectID string, in askUserArgs) *mcp.CallToolResult {
	r, ch := svc.ask.Open(ask.Request{
		SessionID:    sessionID,
		SessionLabel: svc.store.SessionLabel(sessionID),
		Kind:         "choice",
		Title:        in.Question,
		Options:      in.Options,
	})
	svc.bus.Publish(bus.Event{Type: "ask.requested", Payload: r})

	answer, ok := svc.ask.Wait(ctx, ch)
	svc.ask.Remove(r.ID) // no-op se já resolvido
	if !ok {
		// Cancelado (cliente desistiu): o responder não publicou nada, então
		// somos nós a limpar o balão. No caminho de resposta normal, quem publica
		// ask.resolved é handleAskRespond — evita evento duplicado.
		svc.bus.Publish(bus.Event{Type: "ask.resolved", Payload: map[string]any{"request_id": r.ID}})
		return errResult("pergunta cancelada (sem resposta)")
	}
	return textResult(answer)
}
