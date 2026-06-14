package httpapi

import "net/http"

func (s *Server) routesSweep() {
	s.mux.HandleFunc("POST /api/sweep", func(w http.ResponseWriter, r *http.Request) {
		if s.deps.Distiller == nil {
			writeErr(w, http.StatusServiceUnavailable, "varredura indisponível")
			return
		}
		res, err := s.deps.Distiller.Sweep(r.Context())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
}
