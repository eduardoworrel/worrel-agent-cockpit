package store

import (
	"database/sql"
	"testing"
)

func TestSkillCRUD(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sk, err := s.CreateSkill(p.ID, "Deploy Staging", "# conteúdo")
	if err != nil {
		t.Fatal(err)
	}
	if sk.Slug != "deploy-staging" {
		t.Fatalf("slug = %q", sk.Slug)
	}
	if err := s.UpdateSkill(sk.ID, "Deploy Staging", "# v2"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetSkill(sk.ID)
	if got.Content != "# v2" {
		t.Fatalf("content = %q", got.Content)
	}
	all, _ := s.ListSkills("")
	proj, _ := s.ListSkills(p.ID)
	if len(all) != 1 || len(proj) != 1 {
		t.Fatalf("listas: all=%d proj=%d", len(all), len(proj))
	}
	if err := s.DeleteSkill(sk.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSkillSlugCollision(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	a, err := s.CreateSkill(p.ID, "Deploy", "# a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateSkill(p.ID, "Deploy", "# b")
	if err != nil {
		t.Fatal(err)
	}
	if a.Slug == b.Slug {
		t.Fatalf("slugs iguais: %q", a.Slug)
	}
	if b.Slug != "deploy-2" {
		t.Fatalf("slug = %q, want deploy-2", b.Slug)
	}
}

func TestSkillNotFound(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpdateSkill("bogus-id", "nome", "# x"); err != sql.ErrNoRows {
		t.Fatalf("UpdateSkill err = %v, want sql.ErrNoRows", err)
	}
	if err := s.DeleteSkill("bogus-id"); err != sql.ErrNoRows {
		t.Fatalf("DeleteSkill err = %v, want sql.ErrNoRows", err)
	}
}
