package httpapi

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// pasteMaxBytes limita o tamanho da imagem colada no terminal (10MB).
const pasteMaxBytes = 10 << 20

// pasteExtByType mapeia o Content-Type da imagem para a extensão do arquivo.
var pasteExtByType = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/jpg":  ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
	"image/bmp":  ".bmp",
}

// handlePasteImage recebe os bytes de uma imagem colada no terminal web, salva
// em <workspace>/.worrel-pastes/ e devolve o caminho absoluto. O front injeta
// esse caminho no stdin do PTY para a CLI (ex. claude-code) anexar a imagem —
// o clipboard do browser não chega ao processo do servidor de outra forma.
func (s *Server) handlePasteImage(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	sess, err := s.deps.Store.GetSession(sessionID)
	if err != nil {
		notFoundOr500(w, err, "sessão não encontrada")
		return
	}
	if sess.WorkspaceDir == "" {
		writeErr(w, 400, "sessão sem workspace")
		return
	}

	ext, ok := pasteExtByType[r.Header.Get("Content-Type")]
	if !ok {
		writeErr(w, 415, "tipo de imagem não suportado")
		return
	}

	data, err := io.ReadAll(io.LimitReader(r.Body, pasteMaxBytes+1))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if len(data) == 0 {
		writeErr(w, 400, "imagem vazia")
		return
	}
	if len(data) > pasteMaxBytes {
		writeErr(w, 413, "imagem maior que 10MB")
		return
	}

	dir := filepath.Join(sess.WorkspaceDir, ".worrel-pastes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	name := fmt.Sprintf("paste-%d%s", time.Now().UnixNano(), ext)
	full := filepath.Join(dir, name)
	if err := os.WriteFile(full, data, 0o644); err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	writeJSON(w, 201, map[string]string{"path": full})
}
