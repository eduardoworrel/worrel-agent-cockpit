package httpapi

import (
	"net/http"
)

// routesChat registra os endpoints do Chat de Destilação.
//
// Requer o campo Deps.Chat *chat.Service (a fiação é adicionada no server.go /
// main.go pelo dono do projeto). Os handlers só usam s.deps.Chat.
func (s *Server) routesChat() {
	s.mux.HandleFunc("POST /api/chat/threads", func(w http.ResponseWriter, r *http.Request) {
		in, _ := decode[struct {
			Scope    string `json:"scope"`
			Provider string `json:"provider"`
			Model    string `json:"model"`
			Title    string `json:"title"`
		}](r)
		t, err := s.deps.Store.CreateChatThread(in.Scope, in.Provider, in.Model, in.Title)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 201, t)
	})

	s.mux.HandleFunc("GET /api/chat/threads", func(w http.ResponseWriter, r *http.Request) {
		list, err := s.deps.Store.ListChatThreads()
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, list)
	})

	s.mux.HandleFunc("GET /api/chat/threads/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		thread, err := s.deps.Store.GetChatThread(id)
		if err != nil {
			notFoundOr500(w, err, "thread não encontrado")
			return
		}
		msgs, err := s.deps.Store.ListChatMessages(id)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"thread": thread, "messages": msgs})
	})

	s.mux.HandleFunc("POST /api/chat/threads/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		if s.deps.Chat == nil {
			writeErr(w, 503, "chat indisponível")
			return
		}
		id := r.PathValue("id")
		in, err := decode[struct {
			Text     string `json:"text"`
			Provider string `json:"provider"`
			Model    string `json:"model"`
		}](r)
		if err != nil || in.Text == "" {
			writeErr(w, 400, "text obrigatório")
			return
		}
		assistant, sources, created, err := s.deps.Chat.SendMessage(
			r.Context(), id, in.Text, in.Provider, in.Model)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{
			"assistant":           assistant,
			"sources":             sources,
			"created_suggestions": created,
		})
	})
}
