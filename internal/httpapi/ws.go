package httpapi

import (
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
)

// upgrader é compartilhado por /api/events e /api/sessions/{id}/term.
// CheckOrigin aceita apenas clientes locais: Origin vazio (clientes
// não-browser, ex. testes/CLIs) ou host 127.0.0.1/localhost/::1 em
// qualquer porta. Qualquer outra origem (página externa tentando atingir
// o servidor local via browser) é rejeitada.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		switch u.Hostname() {
		case "127.0.0.1", "localhost", "::1":
			return true
		}
		return false
	},
}

func (s *Server) routesWS() {
	s.mux.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		ch, cancel := s.deps.Bus.Subscribe()
		defer cancel()
		done := make(chan struct{})
		go func() { // detect client disconnect
			defer close(done)
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()
		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					return
				}
				if err := conn.WriteJSON(ev); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	})
}
