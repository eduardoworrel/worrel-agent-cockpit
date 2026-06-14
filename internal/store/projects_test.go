package store

import (
	"database/sql"
	"testing"
)

func TestProjectCRUD(t *testing.T) {
	s := newTestStore(t)
	p, err := s.CreateProject("Meu App", "descrição")
	if err != nil {
		t.Fatal(err)
	}
	if p.Slug != "meu-app" {
		t.Fatalf("slug = %q", p.Slug)
	}
	if err := s.AddProjectDir(p.ID, "/tmp/x"); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetProject(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Meu App" || len(got.Dirs) != 1 {
		t.Fatalf("got %+v", got)
	}
	if err := s.UpdateProject(p.ID, "Novo", "d2"); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "Novo" {
		t.Fatalf("list %+v", list)
	}
	if _, err := s.ProjectByDir("/tmp/x"); err != nil {
		t.Fatal(err)
	}
}

func TestSlugCollision(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.CreateProject("App", "")
	b, err := s.CreateProject("App", "")
	if err != nil {
		t.Fatal(err)
	}
	if a.Slug == b.Slug {
		t.Fatalf("slugs iguais: %q", a.Slug)
	}
}

func TestUpdateProjectNotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.UpdateProject("bogus-id", "nome", "desc")
	if err != sql.ErrNoRows {
		t.Fatalf("err = %v, want sql.ErrNoRows", err)
	}
}
