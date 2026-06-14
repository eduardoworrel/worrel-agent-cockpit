package store

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PipelineStep é uma etapa de uma skill composta (pipeline): aponta para
// outra skill (SkillID) e carrega instruções de execução.
type PipelineStep struct {
	SkillID     string `json:"skill_id"`
	Note        string `json:"note"`
	Inputs      string `json:"inputs"`
	Credentials string `json:"credentials"`
}

// pipelineMetadata é o JSON gravado em skills.metadata p/ skills compostas.
type pipelineMetadata struct {
	Kind  string         `json:"kind"`
	Steps []PipelineStep `json:"steps"`
}

// ListPipelines lista as skills cujo metadata marca kind=pipeline.
func (s *Store) ListPipelines(projectID string) ([]*Skill, error) {
	q := `SELECT id, project_id, slug, name, content, created_at, updated_at,
		COALESCE(active_generation,1), COALESCE(evolution_policy,'manual'), COALESCE(origin,'learned'),
		COALESCE(metadata,'{}') FROM skills WHERE metadata LIKE '%"kind":"pipeline"%'`
	args := []any{}
	if projectID != "" {
		q += ` AND project_id=?`
		args = append(args, projectID)
	}
	q += ` ORDER BY updated_at DESC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Skill{}
	for rows.Next() {
		sk := &Skill{}
		if err := rows.Scan(&sk.ID, &sk.ProjectID, &sk.Slug, &sk.Name, &sk.Content,
			&sk.CreatedAt, &sk.UpdatedAt, &sk.ActiveGeneration, &sk.EvolutionPolicy, &sk.Origin,
			&sk.Metadata); err != nil {
			return nil, err
		}
		// O LIKE é uma pré-filtragem barata; confirma via parse.
		var md pipelineMetadata
		if json.Unmarshal([]byte(sk.Metadata), &md) == nil && md.Kind == "pipeline" {
			out = append(out, sk)
		}
	}
	return out, rows.Err()
}

// RenderPipelineMarkdown gera o conteúdo markdown de uma pipeline com etapas
// numeradas. Função pura (testável) — resolve nomes via GetSkill quando o
// store é fornecido; cai para o próprio SkillID quando a skill não existir.
func RenderPipelineMarkdown(s *Store, name string, steps []PipelineStep) string {
	var b strings.Builder
	b.WriteString("# " + name + "\n\n")
	b.WriteString("Pipeline composta de " + fmt.Sprintf("%d", len(steps)) + " etapa(s).\n\n")
	for i, st := range steps {
		target := st.SkillID
		if s != nil {
			if sk, err := s.GetSkill(st.SkillID); err == nil {
				target = sk.Name
			}
		}
		b.WriteString(fmt.Sprintf("## Etapa %d: %s\n\n", i+1, target))
		if st.Note != "" {
			b.WriteString(st.Note + "\n\n")
		}
		if st.Inputs != "" {
			b.WriteString("Entrada: " + st.Inputs + "\n\n")
		}
		if st.Credentials != "" {
			b.WriteString("Credenciais: " + st.Credentials + "\n\n")
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// validateSteps garante que cada etapa aponta para uma skill existente.
func (s *Store) validateSteps(steps []PipelineStep) error {
	if len(steps) < 2 {
		return fmt.Errorf("pipeline exige pelo menos 2 etapas válidas")
	}
	for i, st := range steps {
		if st.SkillID == "" {
			return fmt.Errorf("etapa %d sem skill_id", i+1)
		}
		if _, err := s.GetSkill(st.SkillID); err != nil {
			return fmt.Errorf("etapa %d: skill %q não encontrada", i+1, st.SkillID)
		}
	}
	return nil
}

// CreatePipeline cria uma skill-pipeline: valida as etapas, gera o markdown e
// grava o metadata kind=pipeline.
func (s *Store) CreatePipeline(projectID, name string, steps []PipelineStep) (*Skill, error) {
	if err := s.validateSteps(steps); err != nil {
		return nil, err
	}
	content := RenderPipelineMarkdown(s, name, steps)
	sk, err := s.CreateSkill(projectID, name, content)
	if err != nil {
		return nil, err
	}
	md, err := json.Marshal(pipelineMetadata{Kind: "pipeline", Steps: steps})
	if err != nil {
		return nil, err
	}
	if err := s.SetSkillMetadata(sk.ID, string(md)); err != nil {
		return nil, err
	}
	sk.Metadata = string(md)
	return sk, nil
}

// UpdatePipeline revalida as etapas e regenera content + metadata.
func (s *Store) UpdatePipeline(skillID, name string, steps []PipelineStep) error {
	if err := s.validateSteps(steps); err != nil {
		return err
	}
	content := RenderPipelineMarkdown(s, name, steps)
	if err := s.UpdateSkill(skillID, name, content); err != nil {
		return err
	}
	md, err := json.Marshal(pipelineMetadata{Kind: "pipeline", Steps: steps})
	if err != nil {
		return err
	}
	return s.SetSkillMetadata(skillID, string(md))
}
