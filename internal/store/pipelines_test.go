package store

import (
	"strings"
	"testing"
)

func mkSkills(t *testing.T, s *Store, projectID string, n int) []*Skill {
	t.Helper()
	out := make([]*Skill, 0, n)
	names := []string{"Coletar dados", "Gerar relatório", "Enviar e-mail"}
	for i := 0; i < n; i++ {
		sk, err := s.CreateSkill(projectID, names[i], "# "+names[i])
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, sk)
	}
	return out
}

func TestCreatePipeline(t *testing.T) {
	s := newTestStore(t)
	p, err := s.CreateProject("App", "")
	if err != nil {
		t.Fatal(err)
	}
	sks := mkSkills(t, s, p.ID, 2)
	steps := []PipelineStep{
		{SkillID: sks[0].ID, Note: "primeiro", Inputs: "csv", Credentials: "token-A"},
		{SkillID: sks[1].ID, Note: "depois"},
	}
	pipe, err := s.CreatePipeline(p.ID, "Fluxo", steps)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pipe.Metadata, `"kind":"pipeline"`) {
		t.Fatalf("metadata sem kind=pipeline: %s", pipe.Metadata)
	}
	if !strings.Contains(pipe.Content, "Coletar dados") || !strings.Contains(pipe.Content, "Gerar relatório") {
		t.Fatalf("content sem as 2 etapas: %s", pipe.Content)
	}
	if !strings.Contains(pipe.Content, "Entrada: csv") || !strings.Contains(pipe.Content, "Credenciais: token-A") {
		t.Fatalf("content sem inputs/credenciais: %s", pipe.Content)
	}

	list, err := s.ListPipelines(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != pipe.ID {
		t.Fatalf("ListPipelines = %d, want 1 com id %s", len(list), pipe.ID)
	}
}

func TestCreatePipelineOrphanRejected(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sks := mkSkills(t, s, p.ID, 1)
	// uma válida + uma órfã = 1 válida → falha (<2)
	steps := []PipelineStep{
		{SkillID: sks[0].ID},
		{SkillID: "nao-existe"},
	}
	if _, err := s.CreatePipeline(p.ID, "Fluxo", steps); err == nil {
		t.Fatal("esperava erro com etapa órfã reduzindo a <2 válidas")
	}
}

func TestCreatePipelineRequiresTwo(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sks := mkSkills(t, s, p.ID, 1)
	if _, err := s.CreatePipeline(p.ID, "Fluxo", []PipelineStep{{SkillID: sks[0].ID}}); err == nil {
		t.Fatal("esperava erro com <2 etapas")
	}
}

func TestUpdatePipeline(t *testing.T) {
	s := newTestStore(t)
	p, _ := s.CreateProject("App", "")
	sks := mkSkills(t, s, p.ID, 3)
	pipe, err := s.CreatePipeline(p.ID, "Fluxo", []PipelineStep{
		{SkillID: sks[0].ID}, {SkillID: sks[1].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdatePipeline(pipe.ID, "Fluxo v2", []PipelineStep{
		{SkillID: sks[1].ID}, {SkillID: sks[2].ID},
	}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetSkill(pipe.ID)
	if got.Name != "Fluxo v2" || !strings.Contains(got.Content, "Enviar e-mail") {
		t.Fatalf("update não refletiu: %+v", got)
	}
}

func TestRenderPipelineMarkdownPure(t *testing.T) {
	steps := []PipelineStep{
		{SkillID: "sk-1", Note: "faça X", Inputs: "doc", Credentials: "key"},
		{SkillID: "sk-2"},
	}
	md := RenderPipelineMarkdown(nil, "Meu Fluxo", steps)
	for _, want := range []string{"# Meu Fluxo", "Etapa 1: sk-1", "faça X", "Entrada: doc", "Credenciais: key", "Etapa 2: sk-2"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown sem %q:\n%s", want, md)
		}
	}
}
