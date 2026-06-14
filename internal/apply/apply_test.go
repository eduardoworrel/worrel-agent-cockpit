package apply

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/mirror"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func setup(t *testing.T) (*Applier, *store.Store) {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return New(s, mirror.New(t.TempDir()), bus.New()), s
}

func TestApplyCreateSkill(t *testing.T) {
	a, s := setup(t)
	p, err := s.CreateProject("App", "")
	if err != nil {
		t.Fatal(err)
	}
	sg, err := s.CreateSuggestion(&store.Suggestion{ProjectID: p.ID, Type: "create_skill",
		Title: "Deploy", Payload: `{"name":"Deploy","content":"# passos"}`})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Accept(sg.ID); err != nil {
		t.Fatal(err)
	}
	skills, err := s.ListSkills(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 || skills[0].Name != "Deploy" {
		t.Fatalf("skills %+v", skills)
	}
	got, err := s.GetSuggestion(sg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "accepted" {
		t.Fatalf("status %q", got.Status)
	}
}

func TestApplyAddMemoryAppends(t *testing.T) {
	a, s := setup(t)
	p, err := s.CreateProject("App", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveMemory(p.ID, "# Memória", "init"); err != nil {
		t.Fatal(err)
	}
	sg, err := s.CreateSuggestion(&store.Suggestion{ProjectID: p.ID, Type: "add_memory",
		Title: "Convenção X", Payload: `{"content":"- Use tabs"}`})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Accept(sg.ID); err != nil {
		t.Fatal(err)
	}
	m, err := s.GetMemory(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.Content, "# Memória") || !strings.Contains(m.Content, "- Use tabs") {
		t.Fatalf("memória: %q", m.Content)
	}
}

func TestApplyUpdateSkill(t *testing.T) {
	a, s := setup(t)
	p, err := s.CreateProject("App", "")
	if err != nil {
		t.Fatal(err)
	}
	sk, err := s.CreateSkill(p.ID, "Deploy", "# v1")
	if err != nil {
		t.Fatal(err)
	}
	sg, err := s.CreateSuggestion(&store.Suggestion{ProjectID: p.ID, Type: "update_skill",
		SkillID: &sk.ID, Title: "Deploy: edge case", Payload: `{"name":"Deploy","content":"# v2"}`})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Accept(sg.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSkill(sk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != "# v2" {
		t.Fatalf("content %q", got.Content)
	}
}

func TestApplyAcceptDeferred(t *testing.T) {
	a, s := setup(t)
	p, err := s.CreateProject("App", "")
	if err != nil {
		t.Fatal(err)
	}
	sg, err := s.CreateSuggestion(&store.Suggestion{ProjectID: p.ID, Type: "create_skill",
		Title: "Deploy", Payload: `{"name":"Deploy","content":"# passos"}`})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ResolveSuggestion(sg.ID, "deferred"); err != nil {
		t.Fatal(err)
	}
	if err := a.Accept(sg.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSuggestion(sg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "accepted" {
		t.Fatalf("status %q, want accepted", got.Status)
	}
}

func TestApplyCreateProject(t *testing.T) {
	a, s := setup(t)
	sg, err := s.CreateSuggestion(&store.Suggestion{Type: "create_project",
		Title: "Novo projeto", Payload: `{"name":"Site","description":"d","dirs":["/tmp/site"]}`})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Accept(sg.ID); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "Site" || len(list[0].Dirs) != 1 {
		t.Fatalf("projects %+v", list)
	}
}

// TestApplyCreateProjectNameFallbackAndSeed: candidato sem `name` (nome no
// título da sugestão) → projeto recebe o título como nome (não fica sem título)
// e a descrição é semeada na memória (contexto p/ sessão nova).
func TestApplyCreateProjectNameFallbackAndSeed(t *testing.T) {
	a, s := setup(t)
	sg, err := s.CreateSuggestion(&store.Suggestion{Type: "create_project",
		Title: "Vela v2", Payload: `{"description":"Stack: Go + Rust CLI"}`})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Accept(sg.ID); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListProjects()
	if len(list) != 1 || list[0].Name != "Vela v2" {
		t.Fatalf("projeto deveria ter nome do título, veio %+v", list)
	}
	mem, _ := s.GetMemory(list[0].ID)
	if mem == nil || mem.Content == "" {
		t.Fatal("memória do projeto deveria ser semeada com a descrição")
	}
}

func TestApplyMirrorFailureDoesNotAbort(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	// raiz do mirror é um ARQUIVO: MkdirAll falha em toda escrita
	badRoot := filepath.Join(t.TempDir(), "notdir")
	if err := os.WriteFile(badRoot, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := New(s, mirror.New(badRoot), bus.New())

	p, err := s.CreateProject("App", "")
	if err != nil {
		t.Fatal(err)
	}
	sg, err := s.CreateSuggestion(&store.Suggestion{ProjectID: p.ID, Type: "add_memory",
		Title: "Convenção X", Payload: `{"content":"- Use tabs"}`})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Accept(sg.ID); err != nil {
		t.Fatalf("Accept deve ignorar falha do mirror, err = %v", err)
	}
	got, err := s.GetSuggestion(sg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "accepted" {
		t.Fatalf("status %q, want accepted", got.Status)
	}
	m, err := s.GetMemory(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(m.Content, "- Use tabs") != 1 {
		t.Fatalf("memória deve conter o conteúdo exatamente uma vez: %q", m.Content)
	}
}

func TestAcceptAlreadyAcceptedReturnsError(t *testing.T) {
	a, s := setup(t)
	p, err := s.CreateProject("App", "")
	if err != nil {
		t.Fatal(err)
	}
	sg, err := s.CreateSuggestion(&store.Suggestion{ProjectID: p.ID, Type: "create_skill",
		Title: "Deploy", Payload: `{"name":"Deploy","content":"# passos"}`})
	if err != nil {
		t.Fatal(err)
	}
	// First accept should succeed
	if err := a.Accept(sg.ID); err != nil {
		t.Fatal(err)
	}
	// Second accept should fail with ErrAlreadyResolved
	err = a.Accept(sg.ID)
	if err == nil {
		t.Fatal("expected error on second accept, got nil")
	}
	if !errors.Is(err, ErrAlreadyResolved) {
		t.Fatalf("expected ErrAlreadyResolved, got %v", err)
	}
}
