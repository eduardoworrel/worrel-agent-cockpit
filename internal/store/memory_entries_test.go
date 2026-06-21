package store

import (
	"strings"
	"testing"
)

func TestMemoryEntriesCRUDAndRender(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")

	a, err := s.CreateMemoryEntry(&MemoryEntry{ProjectID: p.ID, Content: "build é go build ./...", Category: "convencao", Evidence: "sess1"})
	if err != nil || a.ID == "" {
		t.Fatalf("create a: %v", err)
	}
	b, _ := s.CreateMemoryEntry(&MemoryEntry{ProjectID: p.ID, Content: "config fica em internal/x", Category: "arquitetura"})

	act, err := s.ListMemoryEntries(p.ID, false)
	if err != nil || len(act) != 2 {
		t.Fatalf("list active: %d %v", len(act), err)
	}

	// supersede a por b
	if err := s.SupersedeMemoryEntry(a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	act, _ = s.ListMemoryEntries(p.ID, false)
	if len(act) != 1 || act[0].ID != b.ID {
		t.Fatalf("after supersede, active=%d", len(act))
	}
	all, _ := s.ListMemoryEntries(p.ID, true)
	if len(all) != 2 {
		t.Fatalf("include superseded=%d", len(all))
	}

	// render só ativas, agrupadas por categoria
	r, err := s.RenderMemory(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(r, "go build") {
		t.Fatalf("render não deveria conter a entrada superseded: %q", r)
	}
	if !strings.Contains(r, "internal/x") || !strings.Contains(r, "## Arquitetura") {
		t.Fatalf("render faltando entrada ativa/seção: %q", r)
	}
}
