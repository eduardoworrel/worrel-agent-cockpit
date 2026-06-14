// Package skillpkg implementa import/export de skills no padrão Agent Skills (SKILL.md).
package skillpkg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Meta contém os metadados do frontmatter de uma SKILL.md. O padrão aberto
// Agent Skills (spec §8.1) exige ao menos `name` e `description`.
type Meta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version,omitempty"`
	Author      string `json:"author,omitempty"`
	License     string `json:"license,omitempty"`
	Origin      string `json:"origin,omitempty"`
}

// GenSummary resume uma geração na linhagem, para o sidecar cockpit.meta.json.
type GenSummary struct {
	Generation    int64  `json:"generation"`
	EvolutionType string `json:"evolution_type"`
	ChangeSummary string `json:"change_summary,omitempty"`
	Authorship    string `json:"authorship,omitempty"`
}

// Sidecar é o metadado estendido próprio (cockpit.meta.json), que consumidores
// externos do padrão Agent Skills podem ignorar com segurança (spec §8.1).
type Sidecar struct {
	SkillID          string       `json:"skill_id,omitempty"`
	Origin           string       `json:"origin,omitempty"`
	ActiveGeneration int64        `json:"active_generation"`
	Generations      int          `json:"generations"`
	Lineage          []GenSummary `json:"lineage,omitempty"`
}

// Package é a representação em memória de uma SKILL.md + sidecar opcional.
type Package struct {
	Meta    Meta
	Content string
	Sidecar *Sidecar
}

// SidecarFile é o nome do arquivo de metadados estendidos.
const SidecarFile = "cockpit.meta.json"

// Render serializa um Package para o formato SKILL.md com frontmatter YAML.
func Render(p *Package) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("name: %s\n", p.Meta.Name))
	// description é obrigatório no padrão aberto; sempre emitido (vazio se ausente).
	sb.WriteString(fmt.Sprintf("description: %s\n", p.Meta.Description))
	if p.Meta.Version != "" {
		sb.WriteString(fmt.Sprintf("version: %s\n", p.Meta.Version))
	}
	if p.Meta.Author != "" {
		sb.WriteString(fmt.Sprintf("author: %s\n", p.Meta.Author))
	}
	if p.Meta.License != "" {
		sb.WriteString(fmt.Sprintf("license: %s\n", p.Meta.License))
	}
	if p.Meta.Origin != "" {
		sb.WriteString(fmt.Sprintf("origin: %s\n", p.Meta.Origin))
	}
	sb.WriteString("---\n")
	sb.WriteString(p.Content)
	return sb.String()
}

// Parse lê o formato SKILL.md e retorna um Package.
func Parse(raw string) (*Package, error) {
	raw = strings.TrimPrefix(raw, "\xef\xbb\xbf") // strip BOM
	if !strings.HasPrefix(raw, "---") {
		return &Package{Content: raw}, nil
	}
	// find end of frontmatter
	rest := raw[3:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, fmt.Errorf("frontmatter não fechado")
	}
	frontmatter := rest[:end]
	content := strings.TrimPrefix(rest[end+4:], "\n")

	meta := Meta{}
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		switch key {
		case "name":
			meta.Name = val
		case "description":
			meta.Description = val
		case "version":
			meta.Version = val
		case "author":
			meta.Author = val
		case "license":
			meta.License = val
		case "origin":
			meta.Origin = val
		}
	}
	return &Package{Meta: meta, Content: content}, nil
}

// WriteDir escreve um Package em <dir>/<slug>/: SKILL.md (padrão aberto) +
// cockpit.meta.json (sidecar de linhagem, ignorável por consumidores externos).
func WriteDir(dir, slug string, p *Package) error {
	dest := filepath.Join(dir, slug)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dest, "SKILL.md"), []byte(Render(p)), 0o644); err != nil {
		return err
	}
	if p.Sidecar != nil {
		b, err := json.MarshalIndent(p.Sidecar, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dest, SidecarFile), b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// ReadDir lê <dir>/<slug>/SKILL.md e retorna um Package. O sidecar
// cockpit.meta.json é IGNORADO na importação (spec §8.1).
func ReadDir(dir, slug string) (*Package, error) {
	b, err := os.ReadFile(filepath.Join(dir, slug, "SKILL.md"))
	if err != nil {
		return nil, err
	}
	return Parse(string(b))
}
