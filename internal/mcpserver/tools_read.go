package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type projectArg struct {
	ProjectID string `json:"project_id,omitempty" jsonschema:"project ID; optional if the session is already bound to a project"`
}

type skillArg struct {
	SkillID string `json:"skill_id" jsonschema:"skill ID"`
}

// jsonResult serializa v como JSON indentado; falha de marshal vira errResult.
func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errResult(err.Error()), nil, nil
	}
	return textResult(string(b)), nil, nil
}

func (svc *Service) addReadTools(srv *mcp.Server, a *attribution) {
	mcp.AddTool(srv, &mcp.Tool{Name: "list_projects",
		Description: "Lists all worrel projects (id, slug, name, description, directories)."},
		func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			list, err := svc.store.ListProjects()
			if err != nil {
				return errResult(err.Error()), nil, nil
			}
			return jsonResult(list)
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "get_project", Description: "Shows the details of a project."},
		func(ctx context.Context, req *mcp.CallToolRequest, in projectArg) (*mcp.CallToolResult, any, error) {
			pid := a.resolveProject(in.ProjectID)
			if pid == "" {
				return errResult("project_id obrigatório (sessão sem projeto vinculado)"), nil, nil
			}
			p, err := svc.store.GetProject(pid)
			if err != nil {
				return errResult("projeto não encontrado"), nil, nil
			}
			return jsonResult(p)
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "get_memory",
		Description: "Loads the project memory (Markdown): conventions, decisions, learned corrections."},
		func(ctx context.Context, req *mcp.CallToolRequest, in projectArg) (*mcp.CallToolResult, any, error) {
			pid := a.resolveProject(in.ProjectID)
			if pid == "" {
				return errResult("project_id obrigatório (sessão sem projeto vinculado)"), nil, nil
			}
			m, err := svc.store.GetMemory(pid)
			if err != nil {
				return errResult(err.Error()), nil, nil
			}
			if m.Content == "" {
				return textResult("(memória vazia)"), nil, nil
			}
			return textResult(m.Content), nil, nil
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "list_skills",
		Description: "Lists skills (all, or those of one project): id, name, slug."},
		func(ctx context.Context, req *mcp.CallToolRequest, in projectArg) (*mcp.CallToolResult, any, error) {
			list, err := svc.store.ListSkills(a.resolveProject(in.ProjectID))
			if err != nil {
				return errResult(err.Error()), nil, nil
			}
			type item struct{ ID, ProjectID, Slug, Name string }
			out := make([]item, 0, len(list))
			for _, sk := range list {
				out = append(out, item{sk.ID, sk.ProjectID, sk.Slug, sk.Name})
			}
			return jsonResult(out)
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "get_skill", Description: "Metadata + content of a skill."},
		func(ctx context.Context, req *mcp.CallToolRequest, in skillArg) (*mcp.CallToolResult, any, error) {
			sk, err := svc.store.GetSkill(in.SkillID)
			if err != nil {
				return errResult("skill não encontrada"), nil, nil
			}
			return jsonResult(sk)
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "load_skill",
		Description: "Loads the full playbook of a skill for execution. Follow the playbook: request the declared inputs, execute the steps, handle the listed edge cases and honor the completion criteria."},
		func(ctx context.Context, req *mcp.CallToolRequest, in skillArg) (*mcp.CallToolResult, any, error) {
			sk, err := svc.store.GetSkill(in.SkillID)
			if err != nil {
				return errResult("skill não encontrada"), nil, nil
			}
			// Métricas (spec §4.1): registra o início de um uso desta skill na
			// sessão atual com a geração ativa. Best-effort: ignora erro.
			sid, _ := a.sessionProject()
			_, _ = svc.store.RecordSkillUsageStart(sk.ID, nilIfEmpty(sid), sk.ActiveGeneration)
			return textResult(sk.Content), nil, nil
		})
}
