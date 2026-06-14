// Package mirror exporta memória e skills como arquivos Markdown legíveis
// em <root>/projects/<slug>/, conforme spec §6.2.
package mirror

import (
	"os"
	"path/filepath"
)

type Mirror struct {
	root string
}

func New(root string) *Mirror { return &Mirror{root: root} }

// projectDir resolve o diretório do projeto sob a raiz do espelho.
// Os slugs são sempre produzidos por store.Slugify (apenas [a-z0-9-]),
// portanto não podem escapar da raiz via ".." ou separadores de caminho.
// Chamadores nunca devem passar entrada bruta do usuário como slug.
func (m *Mirror) projectDir(slug string) string {
	return filepath.Join(m.root, "projects", slug)
}

func (m *Mirror) WriteMemory(slug, content string) error {
	dir := m.projectDir(slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "memory.md"), []byte(content), 0o644)
}

func (m *Mirror) WriteSkill(projectSlug, skillSlug, content string) error {
	dir := filepath.Join(m.projectDir(projectSlug), "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, skillSlug+".md"), []byte(content), 0o644)
}

func (m *Mirror) DeleteSkill(projectSlug, skillSlug string) error {
	err := os.Remove(filepath.Join(m.projectDir(projectSlug), "skills", skillSlug+".md"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
