package skillpkg

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoundtrip(t *testing.T) {
	meta := Meta{
		Name:    "Deploy",
		Version: "1.0.0",
		Author:  "test",
		License: "MIT",
	}
	content := "# Deploy\n\nPassos de deploy."
	pkg := &Package{Meta: meta, Content: content}

	rendered := Render(pkg)
	if rendered == "" {
		t.Fatal("render vazio")
	}

	parsed, err := Parse(rendered)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Meta.Name != meta.Name {
		t.Fatalf("name = %q", parsed.Meta.Name)
	}
	if parsed.Content != content {
		t.Fatalf("content = %q", parsed.Content)
	}
}

func TestParseDescriptionFrontmatter(t *testing.T) {
	md := "---\nname: Deploy Staging\ndescription: deploy ao staging\n---\n# Passos\n1. build\n"
	p, err := Parse(md)
	if err != nil {
		t.Fatal(err)
	}
	if p.Meta.Name != "Deploy Staging" || p.Meta.Description != "deploy ao staging" {
		t.Fatalf("frontmatter: %+v", p.Meta)
	}
}

func TestRenderEmitsDescription(t *testing.T) {
	pkg := &Package{Meta: Meta{Name: "Deploy", Description: "faz deploy"}, Content: "# x"}
	rendered := Render(pkg)
	if !contains(rendered, "description: faz deploy") {
		t.Fatalf("description ausente no render:\n%s", rendered)
	}
	got, _ := Parse(rendered)
	if got.Meta.Description != "faz deploy" {
		t.Fatalf("round-trip description = %q", got.Meta.Description)
	}
}

func TestWriteAndReadDir(t *testing.T) {
	dir := t.TempDir()
	pkg := &Package{
		Meta:    Meta{Name: "Deploy", Description: "d", Version: "1.0.0"},
		Content: "# Deploy steps",
		Sidecar: &Sidecar{ActiveGeneration: 2, Generations: 2, Origin: "learned",
			Lineage: []GenSummary{{Generation: 1, EvolutionType: "learned"}, {Generation: 2, EvolutionType: "correction"}}},
	}
	if err := WriteDir(dir, "deploy", pkg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "deploy", "SKILL.md")); err != nil {
		t.Fatalf("SKILL.md ausente: %v", err)
	}
	// Sidecar cockpit.meta.json deve existir.
	if _, err := os.Stat(filepath.Join(dir, "deploy", SidecarFile)); err != nil {
		t.Fatalf("sidecar ausente: %v", err)
	}
	got, err := ReadDir(dir, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if got.Meta.Name != pkg.Meta.Name || got.Content != pkg.Content {
		t.Fatalf("round-trip: %+v", got)
	}
	// ReadDir ignora o sidecar (não há campo Sidecar populado na importação).
	if got.Sidecar != nil {
		t.Fatal("ReadDir não deve carregar o sidecar (consumidor externo o ignora)")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
