package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/ask"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
)

func (s *Server) routesAsks() {
	s.mux.HandleFunc("POST /api/sessions/{id}/permission-request", s.handlePermissionRequest)
	s.mux.HandleFunc("POST /api/asks/{reqID}/respond", s.handleAskRespond)
	s.mux.HandleFunc("GET /api/asks/pending", s.handleAsksPending)
}

// handlePermissionRequest é chamado pelo hook PreToolUse. Abre um pedido kind=permission,
// publica ask.requested e BLOQUEIA até a UI responder (ou o cliente desistir). Devolve
// {"decision":"allow"|"deny"}.
func (s *Server) handlePermissionRequest(w http.ResponseWriter, r *http.Request) {
	if s.deps.Ask == nil {
		writeErr(w, 503, "ask indisponível")
		return
	}
	sid := r.PathValue("id")
	in, err := decode[struct {
		Tool  string          `json:"tool"`
		Input json.RawMessage `json:"input"`
	}](r)
	if err != nil {
		writeErr(w, 400, "corpo inválido")
		return
	}
	title, detail := toolTitle(in.Tool, in.Input)
	req, ch := s.deps.Ask.Open(ask.Request{
		SessionID:    sid,
		SessionLabel: s.deps.Store.SessionLabel(sid),
		Kind:         "permission",
		Title:        title,
		Detail:       detail,
	})
	s.deps.Bus.Publish(bus.Event{Type: "ask.requested", Payload: req})

	answer, ok := s.deps.Ask.Wait(r.Context(), ch)
	if !ok {
		s.deps.Ask.Remove(req.ID)
		s.deps.Bus.Publish(bus.Event{Type: "ask.resolved", Payload: map[string]any{"request_id": req.ID}})
		writeErr(w, 499, "cancelado")
		return
	}
	writeJSON(w, 200, map[string]string{"decision": answer})
}

func (s *Server) handleAskRespond(w http.ResponseWriter, r *http.Request) {
	if s.deps.Ask == nil {
		writeErr(w, 503, "ask indisponível")
		return
	}
	reqID := r.PathValue("reqID")
	in, err := decode[struct {
		Answer string `json:"answer"`
	}](r)
	if err != nil {
		writeErr(w, 400, "corpo inválido")
		return
	}
	if !s.deps.Ask.Resolve(reqID, in.Answer) {
		writeErr(w, 404, "pedido inexistente ou já resolvido")
		return
	}
	s.deps.Bus.Publish(bus.Event{Type: "ask.resolved", Payload: map[string]any{"request_id": reqID}})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handleAsksPending(w http.ResponseWriter, r *http.Request) {
	if s.deps.Ask == nil {
		writeJSON(w, 200, []ask.Request{})
		return
	}
	writeJSON(w, 200, s.deps.Ask.Pending())
}

// toolTitle traduz (tool, input) do PreToolUse num (título, detalhe) legível.
func toolTitle(tool string, input json.RawMessage) (string, string) {
	var m map[string]any
	_ = json.Unmarshal(input, &m)
	str := func(k string) string { v, _ := m[k].(string); return v }
	switch tool {
	case "Bash":
		return "Rodar comando", str("command")
	case "Edit", "Write", "MultiEdit", "NotebookEdit":
		return "Editar " + str("file_path"), ""
	case "WebFetch":
		return "Acessar " + str("url"), ""
	default:
		return tool, string(input)
	}
}
