package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

type markAsSkillArg struct {
	Title string `json:"title,omitempty" jsonschema:"título curto do fluxo que o usuário quer guardar como skill"`
}

// markAsSkill creates or updates a skill candidate with explicit_mark=1.
func (svc *Service) markAsSkill(sessID, projID, title string) (*store.SkillCandidate, error) {
	if title == "" {
		title = "Fluxo marcado pelo usuário"
	}
	sig := "explicit:" + sessID
	return svc.store.MarkCandidateExplicit(projID, sig, title, "{}",
		store.CandidateOccurrence{SessionID: sessID, Signal: "explicit"})
}

func (svc *Service) addMarkAsSkillTools(srv *mcp.Server, a *attribution) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "mark_as_skill",
		Description: "Marca explicitamente o fluxo da sessão atual como candidato a skill/agente (matura na próxima rodada do motor).",
	},
		func(ctx context.Context, req *mcp.CallToolRequest, in markAsSkillArg) (*mcp.CallToolResult, any, error) {
			sessID, projID := a.sessionProject()
			if projID == "" {
				return errResult("sessão sem projeto vinculado"), nil, nil
			}
			_, err := svc.markAsSkill(sessID, projID, in.Title)
			if err != nil {
				return errResult(err.Error()), nil, nil
			}
			return textResult("fluxo marcado como candidato a skill"), nil, nil
		})
}
