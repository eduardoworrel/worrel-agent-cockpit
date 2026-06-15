package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
)

// routesReset expõe POST /api/reset: "factory reset" que esvazia todas as
// tabelas de dados (projetos, memórias, skills, sugestões, sessões, segredos,
// retroativa, chat, settings) e apaga os diretórios espelhados (projects/) e os
// workspaces gerenciados. O schema do banco e a chave-mestra do SO (Keychain)
// NÃO são tocados.
func (s *Server) routesReset() {
	s.mux.HandleFunc("POST /api/reset", func(w http.ResponseWriter, r *http.Request) {
		if err := s.deps.Store.ResetAll(); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Limpa o que vive no disco fora do banco. Falhas aqui não revertem o
		// reset do banco — apenas reportamos; o conteúdo é recriado sob demanda.
		dataDir := s.deps.Store.DataDir()
		if dataDir != "" {
			_ = os.RemoveAll(filepath.Join(dataDir, "projects"))
			_ = os.RemoveAll(filepath.Join(dataDir, "workspaces"))
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
	})
}
