package httpapi

import (
	"net/http"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func (s *Server) routesSecrets() {
	// --- listagem e criação ---
	s.mux.HandleFunc("GET /api/projects/{id}/secrets", s.listSecrets)
	s.mux.HandleFunc("POST /api/projects/{id}/secrets", s.createSecret)

	// --- operações sobre um segredo ---
	s.mux.HandleFunc("GET /api/secrets/{id}", s.getSecret)
	s.mux.HandleFunc("DELETE /api/secrets/{id}", s.deleteSecret)
	s.mux.HandleFunc("PUT /api/secrets/{id}/value", s.updateSecretValue)
	s.mux.HandleFunc("PUT /api/secrets/{id}/recipe", s.updateSecretRecipe)
	s.mux.HandleFunc("PUT /api/secrets/{id}/policy", s.updateSecretPolicy)

	// --- auditoria ---
	s.mux.HandleFunc("GET /api/projects/{id}/secrets/audit", s.listSecretAudit)

	// --- aprovação via UI ---
	s.mux.HandleFunc("POST /api/secret-approvals/{reqID}", s.approveSecret)

	// --- injeção ---
	s.mux.HandleFunc("PUT /api/projects/{id}/secrets/injection", s.setInjection)

	// --- globais ---
	s.mux.HandleFunc("GET /api/secrets", s.listGlobalSecrets)
	s.mux.HandleFunc("POST /api/secrets", s.createGlobalSecret)
}

func (s *Server) listSecrets(w http.ResponseWriter, r *http.Request) {
	pid := r.PathValue("id")
	list, err := s.deps.Store.ListSecrets(pid)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) listGlobalSecrets(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Store.ListSecrets("")
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, list)
}

type createSecretIn struct {
	Name       string `json:"name"`
	Mode       string `json:"mode"`
	PlainValue string `json:"value,omitempty"`
	Recipe     string `json:"recipe,omitempty"`
	Policy     string `json:"policy,omitempty"`
	Injectable bool   `json:"injectable"`
}

func (s *Server) createSecret(w http.ResponseWriter, r *http.Request) {
	pid := r.PathValue("id")
	s.doCreateSecret(w, r, pid)
}

func (s *Server) createGlobalSecret(w http.ResponseWriter, r *http.Request) {
	s.doCreateSecret(w, r, "")
}

func (s *Server) doCreateSecret(w http.ResponseWriter, r *http.Request, projectID string) {
	in, err := decode[createSecretIn](r)
	if err != nil || in.Name == "" || in.Mode == "" {
		writeErr(w, 400, "name e mode obrigatórios")
		return
	}
	sec := &store.Secret{
		ProjectID:  projectID,
		Name:       in.Name,
		Mode:       in.Mode,
		Recipe:     in.Recipe,
		Policy:     in.Policy,
		Injectable: in.Injectable,
	}
	var ct []byte
	if in.Mode == "value" && in.PlainValue != "" {
		if s.deps.Vault == nil {
			writeErr(w, 503, "cofre indisponível")
			return
		}
		ct, err = s.deps.Vault.Encrypt([]byte(in.PlainValue))
		if err != nil {
			writeErr(w, 500, "erro ao cifrar: "+err.Error())
			return
		}
	}
	created, err := s.deps.Store.CreateSecret(sec, ct)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// Espelha receita na memória do projeto para contexto do agente.
	if in.Mode == "recipe" && in.Recipe != "" && projectID != "" {
		go s.appendRecipeToMemory(projectID, in.Name, in.Recipe)
	}
	writeJSON(w, 201, created)
}

func (s *Server) getSecret(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sec, err := s.deps.Store.GetSecret(id)
	if err != nil {
		notFoundOr500(w, err, "segredo não encontrado")
		return
	}
	writeJSON(w, 200, sec)
}

func (s *Server) deleteSecret(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.deps.Store.DeleteSecret(id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

func (s *Server) updateSecretValue(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	in, err := decode[struct {
		Value string `json:"value"`
	}](r)
	if err != nil || in.Value == "" {
		writeErr(w, 400, "value obrigatório")
		return
	}
	if s.deps.Vault == nil {
		writeErr(w, 503, "cofre indisponível")
		return
	}
	ct, err := s.deps.Vault.Encrypt([]byte(in.Value))
	if err != nil {
		writeErr(w, 500, "erro ao cifrar")
		return
	}
	if err := s.deps.Store.UpdateSecretValue(id, ct); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

func (s *Server) updateSecretRecipe(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	in, err := decode[struct {
		Recipe string `json:"recipe"`
	}](r)
	if err != nil || in.Recipe == "" {
		writeErr(w, 400, "recipe obrigatório")
		return
	}
	if err := s.deps.Store.UpdateSecretRecipe(id, in.Recipe); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

func (s *Server) updateSecretPolicy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	in, err := decode[struct {
		Policy     string `json:"policy"`
		Injectable bool   `json:"injectable"`
	}](r)
	if err != nil || in.Policy == "" {
		writeErr(w, 400, "policy obrigatória")
		return
	}
	if err := s.deps.Store.UpdateSecretPolicy(id, in.Policy, in.Injectable); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

func (s *Server) listSecretAudit(w http.ResponseWriter, r *http.Request) {
	pid := r.PathValue("id")
	rows, err := s.deps.Store.ListSecretAudit(pid)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, rows)
}

// approveSecret responde a um pedido de aprovação pendente no broker.
// Rota: POST /api/secret-approvals/{reqID}, corpo {"approve": true/false}.
func (s *Server) approveSecret(w http.ResponseWriter, r *http.Request) {
	reqID := r.PathValue("reqID")
	in, err := decode[struct {
		Approve bool `json:"approve"`
	}](r)
	if err != nil {
		writeErr(w, 400, "corpo inválido")
		return
	}
	if s.deps.Vault == nil {
		writeErr(w, 503, "cofre indisponível")
		return
	}
	if !s.deps.Vault.Broker().Resolve(reqID, in.Approve) {
		writeErr(w, 404, "pedido de aprovação inexistente ou expirado")
		return
	}
	// Publica evento para a UI atualizar o estado.
	s.deps.Bus.Publish(bus.Event{Type: "secret.approval_resolved", Payload: map[string]any{
		"request_id": reqID,
		"approved":   in.Approve,
	}})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) setInjection(w http.ResponseWriter, r *http.Request) {
	pid := r.PathValue("id")
	in, err := decode[struct {
		Enabled bool `json:"enabled"`
	}](r)
	if err != nil {
		writeErr(w, 400, "corpo inválido")
		return
	}
	val := "false"
	if in.Enabled {
		val = "true"
	}
	if err := s.deps.Store.SetSetting("secrets_injection_enabled:"+pid, val); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

