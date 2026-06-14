package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

type reportTaskArgs struct {
	ProjectID string `json:"project_id,omitempty" jsonschema:"project ID; optional if the session is already bound to a project"`
	Summary   string `json:"summary" jsonschema:"short summary of what was completed"`
	Evidence  string `json:"evidence,omitempty" jsonschema:"evidence of the result (files, commands, outputs)"`
	SkillID   string `json:"skill_id,omitempty" jsonschema:"ID of the skill used for this task, if any (records the usage outcome for skill health metrics)"`
	Outcome   string `json:"outcome,omitempty" jsonschema:"outcome of the skill usage: success | error | abandon (default success)"`
}

type reportCorrectionArgs struct {
	ProjectID  string `json:"project_id,omitempty" jsonschema:"project ID; optional if the session is already bound to a project"`
	WhatFailed string `json:"what_failed" jsonschema:"what was attempted and did not work"`
	WhatWorked string `json:"what_worked" jsonschema:"what worked instead"`
}

type proposeSkillArgs struct {
	ProjectID string `json:"project_id,omitempty" jsonschema:"project ID; optional if the session is already bound to a project"`
	Name      string `json:"name" jsonschema:"name of the proposed skill"`
	Draft     string `json:"draft" jsonschema:"draft of the skill playbook in Markdown"`
}

type proposeSkillUpdateArgs struct {
	ProjectID string `json:"project_id,omitempty" jsonschema:"project ID; optional if the session is already bound to a project"`
	SkillID   string `json:"skill_id" jsonschema:"ID of the skill to update"`
	Diff      string `json:"diff" jsonschema:"description of the proposed change"`
}

type appendMemoryArgs struct {
	ProjectID string `json:"project_id,omitempty" jsonschema:"project ID; optional if the session is already bound to a project"`
	Content   string `json:"content" jsonschema:"content (Markdown) to suggest for the project memory"`
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// contentPayload embala Markdown vindo do agente em {"content": ...}.
// Nota de revisão: o conteúdo é controlado pelo usuário/agente e segue
// para a fila de sugestões como texto — consumidores (UI/apply) devem
// tratá-lo como não-confiável ao renderizar. Sem mudança de comportamento.
func contentPayload(content string) string {
	b, _ := json.Marshal(map[string]string{"content": content})
	return string(b)
}

// createSuggestion grava a sugestão (status pending), publica no bus e
// devolve o texto curto de confirmação para o agente.
func (svc *Service) createSuggestion(sg *store.Suggestion) (*mcp.CallToolResult, any, error) {
	sg, err := svc.store.CreateSuggestion(sg)
	if err != nil {
		return errResult(err.Error()), nil, nil
	}
	svc.bus.Publish(bus.Event{Type: "suggestion.created", Payload: sg})
	return textResult(fmt.Sprintf("Sugestão registrada (id %s). O usuário revisará na fila do worrel.", sg.ID)), nil, nil
}

func (svc *Service) addReportTools(srv *mcp.Server, a *attribution) {
	mcp.AddTool(srv, &mcp.Tool{Name: "report_task_completed",
		Description: "Reports the completion of a task (summary + evidence). Call this whenever you complete an identifiable task. Becomes a memory suggestion for the user to review."},
		func(ctx context.Context, req *mcp.CallToolRequest, in reportTaskArgs) (*mcp.CallToolResult, any, error) {
			pid := a.resolveProject(in.ProjectID)
			if pid == "" {
				return errResult("project_id obrigatório (sessão sem projeto vinculado)"), nil, nil
			}
			sid, _ := a.sessionProject()
			// Métricas (spec §4.1): se a tarefa usou uma skill, fecha o desfecho
			// do uso aberto. outcome default = success. Best-effort.
			if in.SkillID != "" {
				outcome := in.Outcome
				if outcome == "" {
					outcome = "success"
				}
				_ = svc.store.CloseSkillUsageBySession(in.SkillID, sid, outcome, 0, false, 0)
			}
			return svc.createSuggestion(&store.Suggestion{
				ProjectID: pid,
				SessionID: nilIfEmpty(sid),
				Type:      "add_memory",
				Title:     truncate("Tarefa: "+in.Summary, 80),
				Payload:   contentPayload("## Tarefa concluída: " + in.Summary + "\n\n" + in.Evidence),
				Evidence:  in.Evidence,
			})
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "report_correction",
		Description: "Reports a learned correction (what failed vs. what worked). Call this when something you tried failed and another approach worked. Becomes a suggestion for the project memory."},
		func(ctx context.Context, req *mcp.CallToolRequest, in reportCorrectionArgs) (*mcp.CallToolResult, any, error) {
			pid := a.resolveProject(in.ProjectID)
			if pid == "" {
				return errResult("project_id obrigatório (sessão sem projeto vinculado)"), nil, nil
			}
			sid, _ := a.sessionProject()
			return svc.createSuggestion(&store.Suggestion{
				ProjectID: pid,
				SessionID: nilIfEmpty(sid),
				Type:      "add_correction",
				Title:     truncate("Correção: "+in.WhatFailed, 80),
				Payload:   contentPayload("- **Não funciona:** " + in.WhatFailed + "\n  **Faça:** " + in.WhatWorked),
				Evidence:  in.WhatFailed + " -> " + in.WhatWorked,
			})
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "propose_skill",
		Description: "Proposes a new skill (reusable playbook) for the project. Call this when you notice you executed a flow that seems recurrent/reusable. Becomes a suggestion for the user to review."},
		func(ctx context.Context, req *mcp.CallToolRequest, in proposeSkillArgs) (*mcp.CallToolResult, any, error) {
			pid := a.resolveProject(in.ProjectID)
			if pid == "" {
				return errResult("project_id obrigatório (sessão sem projeto vinculado)"), nil, nil
			}
			sid, _ := a.sessionProject()
			payload, err := json.Marshal(map[string]string{"name": in.Name, "content": in.Draft})
			if err != nil {
				return errResult(err.Error()), nil, nil
			}
			return svc.createSuggestion(&store.Suggestion{
				ProjectID: pid,
				SessionID: nilIfEmpty(sid),
				Type:      "create_skill",
				Title:     truncate("Skill: "+in.Name, 80),
				Payload:   string(payload),
			})
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "propose_skill_update",
		Description: "Proposes an update to an existing skill. Call this when you execute a variant of an existing skill or find a new edge case. Becomes a suggestion for the user to review."},
		func(ctx context.Context, req *mcp.CallToolRequest, in proposeSkillUpdateArgs) (*mcp.CallToolResult, any, error) {
			sk, err := svc.store.GetSkill(in.SkillID)
			if err != nil {
				return errResult("skill não encontrada"), nil, nil
			}
			pid := a.resolveProject(in.ProjectID)
			if pid == "" {
				pid = sk.ProjectID
			}
			if pid == "" {
				return errResult("project_id obrigatório (sessão sem projeto vinculado)"), nil, nil
			}
			sid, _ := a.sessionProject()
			payload, err := json.Marshal(map[string]string{
				"name":    sk.Name,
				"content": sk.Content + "\n\n<!-- proposta de update -->\n" + in.Diff,
			})
			if err != nil {
				return errResult(err.Error()), nil, nil
			}
			return svc.createSuggestion(&store.Suggestion{
				ProjectID: pid,
				SessionID: nilIfEmpty(sid),
				SkillID:   &sk.ID,
				Type:      "update_skill",
				Title:     truncate("Update skill: "+sk.Name, 80),
				Payload:   string(payload),
				Evidence:  in.Diff,
			})
		})

	mcp.AddTool(srv, &mcp.Tool{Name: "append_memory_suggestion",
		Description: "Suggests a direct addition to the project memory (Markdown). Call this when you learn a durable fact about the project (convention, decision, secret-handling recipe). Becomes a suggestion for the user to review."},
		func(ctx context.Context, req *mcp.CallToolRequest, in appendMemoryArgs) (*mcp.CallToolResult, any, error) {
			pid := a.resolveProject(in.ProjectID)
			if pid == "" {
				return errResult("project_id obrigatório (sessão sem projeto vinculado)"), nil, nil
			}
			sid, _ := a.sessionProject()
			title := in.Content
			if i := strings.IndexByte(title, '\n'); i >= 0 {
				title = title[:i]
			}
			return svc.createSuggestion(&store.Suggestion{
				ProjectID: pid,
				SessionID: nilIfEmpty(sid),
				Type:      "add_memory",
				Title:     truncate(title, 80),
				Payload:   contentPayload(in.Content),
			})
		})
}
