package mirror

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMirrorWrites(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)
	if err := m.WriteMemory("meu-app", "# memória"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "projects", "meu-app", "memory.md"))
	if err != nil || string(b) != "# memória" {
		t.Fatalf("memory.md: %q %v", b, err)
	}
	if err := m.WriteSkill("meu-app", "deploy", "# skill"); err != nil {
		t.Fatal(err)
	}
	b, err = os.ReadFile(filepath.Join(dir, "projects", "meu-app", "skills", "deploy.md"))
	if err != nil || string(b) != "# skill" {
		t.Fatalf("skill: %q %v", b, err)
	}
	if err := m.DeleteSkill("meu-app", "deploy"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "projects", "meu-app", "skills", "deploy.md")); !os.IsNotExist(err) {
		t.Fatal("skill não removida")
	}
}
