package mcpserver

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
)

type getSecretArgs struct {
	Name      string `json:"name" jsonschema:"nome do segredo a obter"`
	ProjectID string `json:"project_id,omitempty" jsonschema:"id do projeto; opcional se a sessão já está vinculada"`
}

func (svc *Service) addSecretTools(srv *mcp.Server, a *attribution) {
	mcp.AddTool(srv, &mcp.Tool{Name: "get_secret",
		Description: "Obtém um segredo do projeto (ou global) pelo nome. Respeita a política " +
			"de aprovação configurada (pode exigir confirmação do usuário) e registra auditoria. " +
			"Para segredos em modo 'receita', devolve a instrução de obtenção."},
		func(ctx context.Context, req *mcp.CallToolRequest, in getSecretArgs) (*mcp.CallToolResult, any, error) {
			sid, _ := a.sessionProject()
			pid := a.resolveProject(in.ProjectID)
			return svc.handleGetSecret(sid, pid, in.Name), nil, nil
		})
}

// handleGetSecret é a lógica testável de get_secret (resolução de escopo, política,
// aprovação, decifra e auditoria). spec §8.1.
func (svc *Service) handleGetSecret(sessionID, projectID, name string) *mcp.CallToolResult {
	if svc.vault == nil {
		return errResult("cofre indisponível")
	}
	sec, err := svc.store.ResolveSecret(projectID, name)
	if err != nil {
		return errResult("segredo não encontrado: " + name)
	}
	pidPtr := nilIfEmpty(sec.ProjectID)
	sidPtr := nilIfEmpty(sessionID)
	_ = svc.store.AuditSecret(sec.ID, sec.Name, sidPtr, pidPtr, "requested", "")

	// Modo receita: instrução, sem custódia do valor.
	if sec.Mode == "recipe" {
		_ = svc.store.AuditSecret(sec.ID, sec.Name, sidPtr, pidPtr, "granted", "recipe")
		return textResult(sec.Recipe)
	}

	// Decide se precisa de aprovação.
	needApproval := true
	switch sec.Policy {
	case "always":
		needApproval = false
	case "per_session":
		if sessionID != "" && svc.store.HasSessionGrant(sessionID, sec.ID) {
			needApproval = false
		}
	case "per_access":
		needApproval = true
	}

	if needApproval {
		if !svc.askApproval(sessionID, projectID, sec.Name) {
			_ = svc.store.AuditSecret(sec.ID, sec.Name, sidPtr, pidPtr, "denied", "")
			return errResult("acesso ao segredo negado pelo usuário")
		}
	}

	ct, err := svc.store.SecretCiphertext(sec.ID)
	if err != nil {
		return errResult("falha ao ler segredo")
	}
	plain, err := svc.vault.Decrypt(ct)
	if err != nil {
		return errResult("falha ao decifrar segredo")
	}
	_ = svc.store.AuditSecret(sec.ID, sec.Name, sidPtr, pidPtr, "granted", "")
	return textResult(string(plain))
}

// askApproval publica o pedido no bus e bloqueia até a UI responder ou expirar.
func (svc *Service) askApproval(sessionID, projectID, name string) bool {
	br := svc.vault.Broker()
	reqID, ch := br.Open()
	svc.bus.Publish(bus.Event{Type: "secret.approval_requested", Payload: map[string]any{
		"request_id": reqID,
		"name":       name,
		"session":    sessionID,
		"project":    projectID,
	}})
	timeout := time.Duration(svc.approvalTimeoutSeconds()) * time.Second
	ok, err := br.Wait(ch, timeout)
	if err != nil {
		sid := nilIfEmpty(sessionID)
		pid := nilIfEmpty(projectID)
		_ = svc.store.AuditSecretByName(name, projectID, sid, pid, "expired", err.Error())
		return false
	}
	return ok
}

func (svc *Service) approvalTimeoutSeconds() int {
	const def = 120
	v := svc.store.GetSetting("secret_approval_timeout_s", "")
	if v == "" {
		return def
	}
	if n := atoiDefault(v, def); n > 0 {
		return n
	}
	return def
}

func atoiDefault(s string, def int) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	return n
}

