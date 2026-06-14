package mcpserver

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type sessionArg struct {
	SessionID string `json:"session_id,omitempty" jsonschema:"session ID; optional if the MCP connection already uses a session token"`
}

func (svc *Service) addSessionTools(srv *mcp.Server, a *attribution) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_session_summary",
		Description: "Returns the summary of the current session (or of an explicit session_id). If no structured summary exists yet, builds a raw digest from the last 50 transcript events.",
	},
		func(ctx context.Context, req *mcp.CallToolRequest, in sessionArg) (*mcp.CallToolResult, any, error) {
			// resolve session id: token > explicit param
			sessID, _ := a.sessionProject()
			if sessID == "" {
				sessID = in.SessionID
			}
			if sessID == "" {
				return errResult("session_id obrigatório (conexão sem token de sessão vinculado)"), nil, nil
			}

			sess, err := svc.store.GetSession(sessID)
			if err != nil {
				return errResult("sessão não encontrada: " + sessID), nil, nil
			}

			if sess.Summary != "" {
				return textResult(sess.Summary), nil, nil
			}

			// Gerar-e-persistir via Generator estruturado (spec §9).
			if svc.summaryGen != nil {
				out, err := svc.summaryGen.GenerateSummary(ctx, sessID)
				if err == nil && out != "" {
					return textResult(out), nil, nil
				}
				// se falhar, degrada para resumo bruto abaixo
			}

			// Build raw summary from transcript events (last 50)
			events, err := svc.store.ListTranscriptEvents(sessID)
			if err != nil {
				log.Printf("mcp: erro ao carregar transcript da sessão %s: %v", sessID, err)
				return errResult("failed to load transcript"), nil, nil
			}

			if len(events) == 0 {
				return textResult("(sessão sem eventos de transcript)"), nil, nil
			}

			// Take last 50
			start := 0
			if len(events) > 50 {
				start = len(events) - 50
			}
			events = events[start:]

			var sb strings.Builder
			sb.WriteString("## Resumo bruto (a geração estruturada chega com o handoff)\n\n")
			sb.WriteString(fmt.Sprintf("Sessão: %s | Projeto: %s | Status: %s\n\n", sess.ID, sess.ProjectID, sess.Status))

			for _, ev := range events {
				content := truncate(ev.Content, 200)
				if content != ev.Content {
					content += "..."
				}
				sb.WriteString(fmt.Sprintf("**[%s/%s]** %s\n\n", ev.Role, ev.Kind, content))
			}

			return textResult(sb.String()), nil, nil
		})
}
