package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// dirEntry é um subdiretório listável.
type dirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// userHome resolve o home do usuário. Permite override via env WORREL_HOME
// (usado em testes) e resolve symlinks quando possível para comparação estável.
func userHome() string {
	if h := os.Getenv("WORREL_HOME"); h != "" {
		return resolveReal(h)
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return resolveReal(h)
}

// resolveReal limpa, torna absoluto e resolve symlinks quando possível.
func resolveReal(p string) string {
	abs, err := filepath.Abs(filepath.Clean(p))
	if err != nil {
		return filepath.Clean(p)
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real
	}
	return abs
}

// withinHome reporta se path está dentro de (ou é igual a) home.
func withinHome(path, home string) bool {
	if home == "" {
		return false
	}
	if path == home {
		return true
	}
	return strings.HasPrefix(path, home+string(os.PathSeparator))
}

func (s *Server) routesFS() {
	s.mux.HandleFunc("GET /api/fs/dirs", func(w http.ResponseWriter, r *http.Request) {
		home := userHome()
		if home == "" {
			writeErr(w, 500, "home do usuário indisponível")
			return
		}

		path := r.URL.Query().Get("path")
		if path == "" {
			path = home
		} else {
			path = resolveReal(path)
		}

		// SEGURANÇA: restringe a navegação ao home do usuário.
		if !withinHome(path, home) {
			writeErr(w, 403, "caminho fora do diretório home")
			return
		}

		fi, err := os.Stat(path)
		if err != nil {
			notFoundOr500(w, err, "diretório não encontrado")
			return
		}
		if !fi.IsDir() {
			writeErr(w, 400, "caminho não é um diretório")
			return
		}

		ents, err := os.ReadDir(path)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		entries := make([]dirEntry, 0, len(ents))
		for _, e := range ents {
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue // ignora dotfiles/ocultos (.git etc.)
			}
			full := filepath.Join(path, name)
			isDir := e.IsDir()
			if !isDir {
				// resolve symlinks que apontam para diretórios
				if e.Type()&os.ModeSymlink != 0 {
					if st, err := os.Stat(full); err == nil && st.IsDir() {
						isDir = true
					}
				}
			}
			if !isDir {
				continue
			}
			entries = append(entries, dirEntry{Name: name, Path: full})
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })

		// parent vazio quando estamos no home (não deixa subir acima dele).
		parent := ""
		if path != home {
			p := filepath.Dir(path)
			if withinHome(p, home) {
				parent = p
			}
		}

		writeJSON(w, 200, map[string]any{
			"path":    path,
			"parent":  parent,
			"home":    home,
			"entries": entries,
		})
	})
}
