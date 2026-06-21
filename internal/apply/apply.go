// Package apply efetiva sugestões aceitas sobre os artefatos
// (projetos, memória, skills) e mantém o espelho em arquivos.
package apply

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/mirror"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

var ErrAlreadyResolved = errors.New("sugestão já resolvida")

type Applier struct {
	store  *store.Store
	mirror *mirror.Mirror
	bus    *bus.Bus
}

// New cria um Applier. O bus pode ser nil (eventos viram no-op).
func New(s *store.Store, m *mirror.Mirror, b *bus.Bus) *Applier {
	return &Applier{store: s, mirror: m, bus: b}
}

// publish emite um evento de linhagem no bus (no-op se bus == nil).
func (a *Applier) publish(typ, skillID string, gen int64, kind string) {
	if a.bus != nil {
		a.bus.Publish(bus.Event{Type: typ, Payload: map[string]any{
			"skill_id": skillID, "generation": gen, "evolution_type": kind,
		}})
	}
}

// publishBus emite um evento genérico no bus (no-op se bus == nil).
func (a *Applier) publishBus(typ string, payload any) {
	if a.bus != nil {
		a.bus.Publish(bus.Event{Type: typ, Payload: payload})
	}
}

type payload struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Content        string   `json:"content"`
	Dirs           []string `json:"dirs"`
	Diff           string   `json:"diff"`
	ChangeSummary  string   `json:"change_summary"`
	Evidence       string   `json:"evidence"`
	Authorship     string   `json:"authorship"`
	ParentSkillIDs []string `json:"parent_skill_ids"`
}

// Accept efetiva a sugestão sobre o store e resolve seu status.
// O store é a fonte da verdade; o espelho em arquivos é um export
// derivado — falha de escrita no mirror vira log, nunca aborta o
// aceite. Abortar após a escrita no store deixaria a sugestão
// pendente e um retry duplicaria conteúdo (ex.: append na memória).
func (a *Applier) Accept(suggestionID string) error {
	sg, err := a.store.GetSuggestion(suggestionID)
	if err != nil {
		return err
	}
	if sg.Status != "pending" && sg.Status != "deferred" {
		return fmt.Errorf("%w: %s (%s)", ErrAlreadyResolved, sg.ID, sg.Status)
	}
	var p payload
	if err := json.Unmarshal([]byte(sg.Payload), &p); err != nil {
		return fmt.Errorf("payload inválido: %w", err)
	}
	switch sg.Type {
	case "create_project":
		// O nome vem em p.Name; candidatos de chat/destilação costumam pôr o nome
		// no título da sugestão (p.Name vazio) — daí o fallback p/ sg.Title, senão
		// o projeto nasce SEM TÍTULO (slug "projeto").
		name := strings.TrimSpace(p.Name)
		if name == "" {
			name = strings.TrimSpace(sg.Title)
		}
		proj, err := a.store.CreateProject(name, p.Description)
		if err != nil {
			return err
		}
		for _, d := range p.Dirs {
			if err := a.store.AddProjectDir(proj.ID, d); err != nil {
				return err
			}
		}
		// Semeia a memória do projeto com a descrição/conteúdo proposto, para que
		// uma sessão nova já tenha CONTEXTO (stack, decisões) no primer — senão o
		// agente abre num workspace vazio sem saber do que se trata.
		seed := strings.TrimSpace(p.Description)
		if seed == "" {
			seed = strings.TrimSpace(p.Content)
		}
		if seed != "" {
			if _, err := a.store.SaveMemory(proj.ID, seed, "criação: "+name); err == nil {
				if err := a.mirror.WriteMemory(proj.Slug, seed); err != nil {
					log.Printf("mirror: %v", err)
				}
			}
		}
	case "add_memory", "add_correction":
		proj, err := a.store.GetProject(sg.ProjectID)
		if err != nil {
			return err
		}
		mem, err := a.store.GetMemory(sg.ProjectID)
		if err != nil {
			return err
		}
		content := mem.Content
		if content != "" {
			content += "\n\n"
		}
		content += p.Content
		if _, err := a.store.SaveMemory(sg.ProjectID, content, "sugestão: "+sg.Title); err != nil {
			return err
		}
		if err := a.mirror.WriteMemory(proj.Slug, content); err != nil {
			log.Printf("mirror: %v", err)
		}
	case "create_skill":
		proj, err := a.store.GetProject(sg.ProjectID)
		if err != nil {
			return err
		}
		sk, err := a.store.CreateSkill(sg.ProjectID, p.Name, p.Content)
		if err != nil {
			return err
		}
		if err := a.mirror.WriteSkill(proj.Slug, sk.Slug, sk.Content); err != nil {
			log.Printf("mirror: %v", err)
		}
		a.publish("skill.generation.created", sk.ID, 1, "learned")
	case "skill.variant":
		// Variante (spec §3, critério 3): cria skill NOVA com identidade própria
		// e regista a(s) skill(s)-mãe na geração 1, reclassificando-a de 'learned'
		// para 'variant'. A(s) mãe(s) permanecem ativas e coexistem.
		proj, err := a.store.GetProject(sg.ProjectID)
		if err != nil {
			return err
		}
		sk, err := a.store.CreateSkill(sg.ProjectID, p.Name, p.Content)
		if err != nil {
			return err
		}
		evidence := p.Evidence
		if evidence == "" {
			evidence = sg.Evidence
		}
		summary := p.ChangeSummary
		if summary == "" {
			summary = sg.Title
		}
		if err := a.store.RewriteSeedAsVariant(sk.ID, p.ParentSkillIDs, evidence, summary); err != nil {
			return err
		}
		if err := a.mirror.WriteSkill(proj.Slug, sk.Slug, sk.Content); err != nil {
			log.Printf("mirror: %v", err)
		}
		a.publish("skill.generation.created", sk.ID, 1, "variant")
	case "update_skill":
		if sg.SkillID == nil {
			return fmt.Errorf("%s sem skill_id", sg.Type)
		}
		if err := a.store.UpdateSkill(*sg.SkillID, p.Name, p.Content); err != nil {
			return err
		}
		sk, err := a.store.GetSkill(*sg.SkillID)
		if err != nil {
			return err
		}
		proj, err := a.store.GetProject(sk.ProjectID)
		if err != nil {
			return err
		}
		if err := a.mirror.WriteSkill(proj.Slug, sk.Slug, sk.Content); err != nil {
			log.Printf("mirror: %v", err)
		}
	case "skill.learned":
		// skill.learned sem skill_id cria nova skill; com skill_id adiciona geração
		if sg.SkillID == nil {
			proj, err := a.store.GetProject(sg.ProjectID)
			if err != nil {
				return err
			}
			sk, err := a.store.CreateSkill(sg.ProjectID, p.Name, p.Content)
			if err != nil {
				return err
			}
			if err := a.mirror.WriteSkill(proj.Slug, sk.Slug, sk.Content); err != nil {
				log.Printf("mirror: %v", err)
			}
			a.publish("skill.generation.created", sk.ID, 1, "learned")
			break
		}
		fallthrough
	case "skill.correction":
		if sg.SkillID == nil {
			return fmt.Errorf("%s sem skill_id", sg.Type)
		}
		evType := "correction"
		if sg.Type == "skill.learned" {
			evType = "learned"
		}
		authorship := p.Authorship
		if authorship == "" {
			authorship = "human"
		}
		evidence := p.Evidence
		if evidence == "" {
			evidence = sg.Evidence
		}
		g, err := a.store.AddGeneration(*sg.SkillID, store.GenerationInput{
			EvolutionType: evType,
			Snapshot:      p.Content,
			Diff:          p.Diff,
			ChangeSummary: p.ChangeSummary,
			Evidence:      evidence,
			Authorship:    authorship,
		})
		if err != nil {
			return err
		}
		sk, err := a.store.GetSkill(*sg.SkillID)
		if err != nil {
			return err
		}
		proj, err := a.store.GetProject(sk.ProjectID)
		if err != nil {
			return err
		}
		if err := a.mirror.WriteSkill(proj.Slug, sk.Slug, sk.Content); err != nil {
			log.Printf("mirror: %v", err)
		}
		a.publish("skill.generation.created", sk.ID, g.Generation, evType)
	case "add_memory_entry":
		if _, err := a.applyMemoryEntry(sg, ""); err != nil {
			return err
		}
	case "pipeline":
		if err := a.ApplyPipeline(sg); err != nil {
			return err
		}
	default:
		return fmt.Errorf("tipo de sugestão desconhecido: %s", sg.Type)
	}
	return a.store.ResolveSuggestion(sg.ID, "accepted")
}

// applyMemoryEntry cria a memory_entry a partir do payload da sugestão. Se
// supersedeOldID != "", marca a entrada antiga como superseded pela nova.
// Devolve o id da nova entrada.
func (a *Applier) applyMemoryEntry(sg *store.Suggestion, supersedeOldID string) (string, error) {
	var p struct {
		Content  string `json:"content"`
		Category string `json:"category"`
		Evidence string `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(sg.Payload), &p); err != nil {
		return "", err
	}
	e, err := a.store.CreateMemoryEntry(&store.MemoryEntry{
		ProjectID: sg.ProjectID, Content: p.Content, Category: p.Category, Evidence: p.Evidence,
	})
	if err != nil {
		return "", err
	}
	if supersedeOldID != "" {
		if err := a.store.SupersedeMemoryEntry(supersedeOldID, e.ID); err != nil {
			return "", err
		}
	}
	return e.ID, nil
}

// AcceptSuperseding aceita uma sugestão add_memory_entry criando a nova entrada e
// marcando oldEntryID como superseded por ela. Só válido para add_memory_entry.
func (a *Applier) AcceptSuperseding(suggestionID, oldEntryID string) error {
	sg, err := a.store.GetSuggestion(suggestionID)
	if err != nil {
		return err
	}
	if sg.Type != "add_memory_entry" {
		return fmt.Errorf("AcceptSuperseding só vale para add_memory_entry, got %q", sg.Type)
	}
	if _, err := a.applyMemoryEntry(sg, oldEntryID); err != nil {
		return err
	}
	return a.store.ResolveSuggestion(sg.ID, "accepted")
}

// AutoApply efetiva uma sugestão automaticamente (engine) e registra como auto_applied.
func (a *Applier) AutoApply(suggestionID string) error {
	sg, err := a.store.GetSuggestion(suggestionID)
	if err != nil {
		return err
	}
	if sg.Status != "pending" && sg.Status != "deferred" {
		return fmt.Errorf("%w: %s (%s)", ErrAlreadyResolved, sg.ID, sg.Status)
	}
	var p payload
	if err := json.Unmarshal([]byte(sg.Payload), &p); err != nil {
		return fmt.Errorf("payload inválido: %w", err)
	}
	// Force authorship to engine_auto
	p.Authorship = "engine_auto"
	if sg.SkillID != nil {
		evType := "correction"
		if sg.Type == "skill.learned" {
			evType = "learned"
		}
		evidence := p.Evidence
		if evidence == "" {
			evidence = sg.Evidence
		}
		if _, err := a.store.AddGeneration(*sg.SkillID, store.GenerationInput{
			EvolutionType: evType,
			Snapshot:      p.Content,
			Diff:          p.Diff,
			ChangeSummary: p.ChangeSummary,
			Evidence:      evidence,
			Authorship:    "engine_auto",
		}); err != nil {
			return err
		}
		sk, err := a.store.GetSkill(*sg.SkillID)
		if err != nil {
			return err
		}
		proj, err := a.store.GetProject(sk.ProjectID)
		if err != nil {
			return err
		}
		if err := a.mirror.WriteSkill(proj.Slug, sk.Slug, sk.Content); err != nil {
			log.Printf("mirror: %v", err)
		}
	}
	return a.store.ResolveSuggestion(sg.ID, "auto_applied")
}
