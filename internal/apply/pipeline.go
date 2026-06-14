package apply

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// pipelinePayload é o formato do payload de uma Suggestion{Type:"pipeline"}:
// {"name":"...","steps":[{"skill_id":"...","note":"...","inputs":"...","credentials":"..."}]}
type pipelinePayload struct {
	Name  string               `json:"name"`
	Steps []store.PipelineStep `json:"steps"`
}

// ApplyPipeline efetiva uma sugestão de pipeline: resolve os skill_ids do
// payload, descarta etapas órfãs (skill inexistente ou sem id) e cria a
// skill-pipeline via store.CreatePipeline (que exige >=2 etapas válidas).
// Não resolve o status da sugestão — o chamador (Accept) faz isso.
func (a *Applier) ApplyPipeline(sg *store.Suggestion) error {
	var p pipelinePayload
	if err := json.Unmarshal([]byte(sg.Payload), &p); err != nil {
		return fmt.Errorf("payload de pipeline inválido: %w", err)
	}
	name := p.Name
	if name == "" {
		name = sg.Title
	}
	valid := make([]store.PipelineStep, 0, len(p.Steps))
	for _, st := range p.Steps {
		if st.SkillID == "" {
			continue
		}
		if _, err := a.store.GetSkill(st.SkillID); err != nil {
			log.Printf("pipeline: descartando etapa órfã skill_id=%q: %v", st.SkillID, err)
			continue
		}
		valid = append(valid, st)
	}
	if len(valid) < 2 {
		return fmt.Errorf("pipeline exige pelo menos 2 etapas válidas, obtidas %d", len(valid))
	}
	sk, err := a.store.CreatePipeline(sg.ProjectID, name, valid)
	if err != nil {
		return err
	}
	if a.mirror != nil {
		if proj, perr := a.store.GetProject(sg.ProjectID); perr == nil {
			if err := a.mirror.WriteSkill(proj.Slug, sk.Slug, sk.Content); err != nil {
				log.Printf("mirror: %v", err)
			}
		}
	}
	a.publish("skill.generation.created", sk.ID, 1, "pipeline")
	return nil
}
