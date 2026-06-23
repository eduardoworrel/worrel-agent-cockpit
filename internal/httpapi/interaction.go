package httpapi

import (
	"net/http"
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/agui"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/ask"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
)

// routesInteraction expõe o canal AG-UI que serve a Home: ler o snapshot de
// interação de uma sessão, responder uma pergunta bloqueante e injetar um novo
// prompt quando a sessão está ociosa. O terminal/PTY (/term) é independente.
func (s *Server) routesInteraction() {
	s.mux.HandleFunc("GET /api/sessions/{id}/interaction", s.handleInteraction)
	s.mux.HandleFunc("POST /api/sessions/{id}/interaction/respond", s.handleInteractionRespond)
	s.mux.HandleFunc("POST /api/sessions/{id}/interaction/prompt", s.handleInteractionPrompt)
	// Fila de adiadas: adiar uma sessão (vira bolinha no sidebar) e listar a fila.
	s.mux.HandleFunc("POST /api/sessions/{id}/defer", s.handleDeferSession)
	s.mux.HandleFunc("GET /api/deferred", s.handleListDeferred)
}

// handleDeferSession marca a sessão como adiada: o modal de interação fecha e a
// sessão vira uma bolinha no sidebar, que reabre o modal ao ser clicada. Não
// reabre sozinha.
func (s *Server) handleDeferSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.deps.Store.SetSessionDeferred(id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.deps.Bus.Publish(bus.Event{Type: "session.deferred", Payload: map[string]any{"session_id": id}})
	w.WriteHeader(204)
}

// handleListDeferred devolve a fila de adiadas (mais recentes primeiro).
func (s *Server) handleListDeferred(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Store.ListDeferredSessions()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, list)
}

// clearDeferred remove a marca de adiada e avisa o front (a bolinha some).
// Chamado ao responder/enviar: responder resolve o pedido, então não faz
// sentido continuar na fila.
func (s *Server) clearDeferred(id string) {
	if err := s.deps.Store.ClearSessionDeferred(id); err == nil {
		s.deps.Bus.Publish(bus.Event{Type: "session.undeferred", Payload: map[string]any{"session_id": id}})
	}
}

// handleInteraction devolve o Snapshot AG-UI atual da sessão (estado, última
// fala da IA, o que ela fez, último pedido do usuário e interrupt pendente).
func (s *Server) handleInteraction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// Sessão do motor: o Snapshot vem direto do stream-json (ao vivo), sem
	// transcript/ask/PTY.
	if s.deps.Engine != nil {
		if snap, ok := s.deps.Engine.Snapshot(id); ok {
			// Auto-mode: se o agente terminou FALANDO (sem permissão pendente),
			// interpreta a fala em opções/resposta via LLM (assíncrono, cacheado).
			s.attachInterpretation(&snap)
			// Título vivo + eventos narrados (card) gerados do histórico.
			s.attachEngineSummary(&snap)
			// Sessões de motor recebem permissões nativas (can_use_tool via
			// stdio), mas perguntas via broker (MCP ask_user) não passam pelo
			// motor. Se não há interrupt nativo e há ask pendente para esta
			// sessão, expõe-o para o modal renderizar a pergunta.
			if snap.Interrupt == nil && s.deps.Ask != nil {
				for _, rq := range s.deps.Ask.Pending() {
					if rq.SessionID == id {
						snap.Interrupt = agui.InterruptFromAsk(rq)
						if snap.State == agui.StateWorking {
							snap.State = agui.StateAwaiting
						}
						break
					}
				}
			}
			writeJSON(w, 200, snap)
			return
		}
	}
	events, err := s.deps.Store.ListTranscriptEvents(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	var pending []ask.Request
	if s.deps.Ask != nil {
		pending = s.deps.Ask.Pending()
	}
	ended := !s.deps.Wrapper.IsRunning(id)
	snap := agui.Build(id, ended, events, pending)
	s.attachProgress(&snap, events)
	writeJSON(w, 200, snap)
}

// handleInteractionRespond responde uma pergunta bloqueante (interrupt) pelo
// request_id — equivale a clicar no balão antigo, mas via contrato AG-UI.
func (s *Server) handleInteractionRespond(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	in, err := decode[struct {
		RequestID string `json:"request_id"`
		Answer    string `json:"answer"`
	}](r)
	if err != nil {
		writeErr(w, 400, "corpo inválido")
		return
	}
	// Ask de broker (hook PreToolUse de PTY ou a tool MCP ask_user) tem
	// prioridade: resolve pelo request_id. É no-op (retorna false) para um
	// interrupt nativo do motor, cujo request_id não vive no broker — aí caímos
	// no caminho do motor abaixo. Isso cobre a sessão de motor que faz ask_user.
	if s.deps.Ask != nil && in.RequestID != "" && s.deps.Ask.Resolve(in.RequestID, in.Answer) {
		s.clearDeferred(id)
		s.deps.Bus.Publish(bus.Event{Type: "ask.resolved", Payload: map[string]any{"request_id": in.RequestID}})
		w.WriteHeader(204)
		return
	}
	// Sessão do motor: a permissão nativa é respondida pelo stream (allow/deny).
	if s.deps.Engine != nil && s.deps.Engine.Has(id) {
		allow := in.Answer == "allow" || in.Answer == "yes" || in.Answer == "permitir"
		if err := s.deps.Engine.Respond(id, allow); err != nil {
			writeErr(w, 409, err.Error())
			return
		}
		s.clearDeferred(id)
		w.WriteHeader(204)
		return
	}
	if s.deps.Ask == nil {
		writeErr(w, 503, "ask indisponível")
		return
	}
	if in.RequestID == "" {
		writeErr(w, 400, "request_id obrigatório")
		return
	}
	writeErr(w, 404, "pedido inexistente ou já resolvido")
}

// handleInteractionPrompt injeta um novo prompt no stdin do PTY quando a sessão
// está ociosa (turno do usuário). É o equivalente, via Home, a digitar no
// terminal e dar Enter — sem precisar abrir o terminal.
func (s *Server) handleInteractionPrompt(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	in, err := decode[struct {
		Text string `json:"text"`
	}](r)
	if err != nil || strings.TrimSpace(in.Text) == "" {
		writeErr(w, 400, "text obrigatório")
		return
	}
	// Sessão do motor: o prompt vai pelo stream-json (stdin), não pelo PTY.
	if s.deps.Engine != nil && s.deps.Engine.Has(id) {
		if err := s.deps.Engine.SendPrompt(id, in.Text); err != nil {
			writeErr(w, 409, err.Error())
			return
		}
		s.clearDeferred(id)
		w.WriteHeader(204)
		return
	}
	if !s.deps.Wrapper.IsRunning(id) {
		writeErr(w, 409, "sessão não está rodando")
		return
	}
	// "\r" reproduz o Enter que o CLI espera no PTY (o terminal envia o mesmo).
	if err := s.deps.Wrapper.Write(id, []byte(in.Text+"\r")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.clearDeferred(id)
	s.deps.Bus.Publish(bus.Event{Type: "session.busy", Payload: map[string]any{"session_id": id}})
	w.WriteHeader(204)
}
