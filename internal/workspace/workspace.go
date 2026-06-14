// Package workspace gerencia o diretório-base de execução de cada escopo
// (projeto). A base é sempre app-managed em <root>/workspaces/<slug>/; as
// pastas reais associadas ao escopo entram como symlinks dentro dela. Isso
// honra "projeto = escopo, não pasta": o cwd de execução é controlado pelo
// app e um escopo pode abranger N pastas.
package workspace

import (
	"os"
	"path/filepath"
)

type Manager struct {
	root string // ex.: ~/.worrel
}

func New(root string) *Manager { return &Manager{root: root} }

func (m *Manager) dir(slug string) string {
	return filepath.Join(m.root, "workspaces", slug)
}

// EnsureWorkspace cria (idempotente) e devolve o diretório-base do escopo.
func (m *Manager) EnsureWorkspace(slug string) (string, error) {
	d := m.dir(slug)
	return d, os.MkdirAll(d, 0o755)
}

// ScratchWorkspace cria um workspace temporário para uma sessão não-classificada.
func (m *Manager) ScratchWorkspace(sessionID string) (string, error) {
	return m.EnsureWorkspace("_scratch-" + sessionID)
}

// SyncSymlinks faz os symlinks dentro de ws refletirem exatamente realPaths.
// Symlinks gerenciados ausentes da lista são removidos; basenames repetidos
// recebem sufixo numérico (api, api-2, ...). Não toca em arquivos que não
// sejam symlinks (segurança). Alvo inexistente ainda vira symlink (ver Broken).
func (m *Manager) SyncSymlinks(ws string, realPaths []string) error {
	want := map[string]string{} // linkName -> target
	used := map[string]bool{}
	for _, rp := range realPaths {
		base := filepath.Base(rp)
		name := base
		for i := 2; used[name]; i++ {
			name = base + "-" + itoa(i)
		}
		used[name] = true
		want[name] = rp
	}
	// remover symlinks gerenciados que não estão mais em want
	entries, err := os.ReadDir(ws)
	if err != nil {
		return err
	}
	for _, e := range entries {
		full := filepath.Join(ws, e.Name())
		fi, lerr := os.Lstat(full)
		if lerr != nil || fi.Mode()&os.ModeSymlink == 0 {
			continue // só mexe em symlinks
		}
		if _, keep := want[e.Name()]; !keep {
			if err := os.Remove(full); err != nil {
				return err
			}
		}
	}
	// criar/atualizar os desejados
	for name, target := range want {
		link := filepath.Join(ws, name)
		if cur, err := os.Readlink(link); err == nil {
			if cur == target {
				continue
			}
			_ = os.Remove(link)
		}
		if err := os.Symlink(target, link); err != nil {
			return err
		}
	}
	return nil
}

// BrokenLinks devolve os nomes de symlinks cujo alvo não existe.
func (m *Manager) BrokenLinks(ws string) []string {
	var out []string
	entries, err := os.ReadDir(ws)
	if err != nil {
		return out
	}
	for _, e := range entries {
		full := filepath.Join(ws, e.Name())
		fi, lerr := os.Lstat(full)
		if lerr != nil || fi.Mode()&os.ModeSymlink == 0 {
			continue
		}
		if _, err := os.Stat(full); err != nil { // segue o link; erro = quebrado
			out = append(out, e.Name())
		}
	}
	return out
}

// RemoveWorkspace apaga o workspace inteiro (usado ao deletar projeto/scratch).
func (m *Manager) RemoveWorkspace(ws string) error {
	err := os.RemoveAll(ws)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		p--
		b[p] = '-'
	}
	return string(b[p:])
}
