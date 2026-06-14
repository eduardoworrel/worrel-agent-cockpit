package chat

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/distill"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// Service implementa o Chat de Destilação: recupera sessões relevantes do
// histórico (dentro do escopo do thread), sane segredos, conversa com um LLM
// headless e converte artefatos identificados em sugestões (origin="chat").
type Service struct {
	store           *store.Store
	headless        map[string]distill.Headless
	defaultHeadless distill.Headless
	bus             *bus.Bus
}

func NewService(st *store.Store, headless map[string]distill.Headless, def distill.Headless, b *bus.Bus) *Service {
	return &Service{store: st, headless: headless, defaultHeadless: def, bus: b}
}

func (svc *Service) headlessFor(provider string) distill.Headless {
	if provider != "" && svc.headless != nil {
		if h, ok := svc.headless[provider]; ok && h != nil {
			return h
		}
	}
	return svc.defaultHeadless
}

func (svc *Service) publish(t string, payload any) {
	if svc.bus != nil {
		svc.bus.Publish(bus.Event{Type: t, Payload: payload})
	}
}

// SendMessage processa uma mensagem do usuário num thread: recupera contexto,
// chama o LLM, persiste user+assistant e cria sugestões a partir dos candidatos.
func (svc *Service) SendMessage(ctx context.Context, threadID, userText, provider, model string) (
	assistant string, sources []SessionRef, createdSuggestionIDs []string, err error) {

	thread, err := svc.store.GetChatThread(threadID)
	if err != nil {
		return "", nil, nil, err
	}
	if provider == "" {
		provider = thread.Provider
	}
	if model == "" {
		model = thread.Model
	}

	history, err := svc.store.ListChatMessages(threadID)
	if err != nil {
		return "", nil, nil, err
	}

	scope := parseScope(thread.Scope)
	retrieved, err := svc.retrieve(scope, userText)
	if err != nil {
		return "", nil, nil, err
	}
	sources = make([]SessionRef, 0, len(retrieved))
	for _, rs := range retrieved {
		sources = append(sources, rs.ref)
	}

	prompt := buildPrompt(history, retrieved, userText)

	out, err := svc.headlessFor(provider).RunHeadless(ctx, prompt, adapter.HeadlessOpts{Model: model})
	if err != nil {
		return "", nil, nil, err
	}

	assistant = extractText(out)
	createdSuggestionIDs = svc.createSuggestions(out, scope)

	// Persiste user + assistant (com sources).
	if _, err := svc.store.AppendChatMessage(threadID, "user", userText, ""); err != nil {
		return "", nil, nil, err
	}
	srcJSON, _ := json.Marshal(sources)
	if _, err := svc.store.AppendChatMessage(threadID, "assistant", assistant, string(srcJSON)); err != nil {
		return "", nil, nil, err
	}

	svc.publish("chat.message", map[string]any{
		"thread_id": threadID, "created_suggestions": len(createdSuggestionIDs),
	})
	return assistant, sources, createdSuggestionIDs, nil
}

// extractText devolve o texto da resposta sem o array JSON final de candidatos.
func extractText(raw string) string {
	s := strings.TrimSpace(raw)
	i := strings.Index(s, "[")
	j := strings.LastIndex(s, "]")
	if i >= 0 && j > i {
		// confirma que o trecho é um array JSON válido antes de removê-lo.
		var probe []json.RawMessage
		if json.Unmarshal([]byte(s[i:j+1]), &probe) == nil {
			s = strings.TrimSpace(s[:i])
			s = strings.TrimSuffix(s, "```json")
			s = strings.TrimSpace(s)
		}
	}
	return s
}

// pipelineCandidate é o contrato fixo do candidato type:"pipeline".
type pipelineCandidate struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	Title     string `json:"title"`
	Steps     []struct {
		SkillID     string `json:"skill_id"`
		Note        string `json:"note"`
		Inputs      string `json:"inputs"`
		Credentials string `json:"credentials"`
	} `json:"steps"`
	Evidence  string `json:"evidence"`
	ProjectID string `json:"project_id"`
}

// createSuggestions parseia os candidatos do output do LLM e os insere como
// sugestões origin="chat": skill/memory/project via distill.ParseCandidates (com
// guardas) e pipeline via o caminho dedicado com validação de etapas.
func (svc *Service) createSuggestions(raw string, scope Scope) []string {
	var ids []string

	// 1) skill.*/add_memory/create_project: reusa o parser+validação do distill.
	cands, _ := distill.ParseCandidates(raw)
	for _, c := range cands {
		if c.ProjectID == "" && scope.ProjectID != "" {
			c.ProjectID = scope.ProjectID
		}
		// Guarda de FK: skill_id citado precisa existir; correction sem alvo cai fora.
		if c.SkillID != "" {
			if _, err := svc.store.GetSkill(c.SkillID); err != nil {
				if c.Type == "skill.correction" {
					continue
				}
				c.SkillID = ""
				c.ParentSkillIDs = nil
			}
		}
		sg := &store.Suggestion{
			ProjectID: c.ProjectID,
			Type:      c.Type,
			Title:     c.Title,
			Payload:   candidatePayload(c),
			Evidence:  c.Evidence,
			Origin:    "chat",
		}
		if c.SkillID != "" {
			sg.SkillID = &c.SkillID
		}
		created, err := svc.store.CreateSuggestion(sg)
		if err != nil {
			log.Printf("chat: falha ao inserir sugestão %s: %v", c.Type, err)
			continue
		}
		ids = append(ids, created.ID)
		svc.publish("suggestion.created", map[string]any{"type": c.Type, "title": c.Title, "origin": "chat"})
	}

	// 2) pipeline: parser dedicado (ParseCandidates ignora type desconhecido).
	for _, p := range parsePipelines(raw) {
		if id, ok := svc.insertPipeline(p, scope); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

// candidatePayload espelha o payload de add_memory/skill do distill (content
// preenchido a partir de description quando vazio).
func candidatePayload(c distill.Candidate) string {
	content := c.Content
	if content == "" && c.Description != "" {
		content = c.Description
	}
	b, _ := json.Marshal(map[string]any{
		"name": c.Name, "content": content, "description": c.Description,
		"parent_skill_ids": c.ParentSkillIDs,
	})
	return string(b)
}

func parsePipelines(raw string) []pipelineCandidate {
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, "["); i >= 0 {
		if j := strings.LastIndex(s, "]"); j > i {
			s = s[i : j+1]
		}
	}
	var items []pipelineCandidate
	if err := json.Unmarshal([]byte(s), &items); err != nil {
		return nil
	}
	out := items[:0]
	for _, it := range items {
		if it.Type == "pipeline" {
			out = append(out, it)
		}
	}
	return out
}

// insertPipeline valida cada etapa (skill_id deve existir; etapas órfãs são
// descartadas) e só cria a sugestão se sobrarem >=2 etapas válidas.
func (svc *Service) insertPipeline(p pipelineCandidate, scope Scope) (string, bool) {
	if strings.TrimSpace(p.Name) == "" {
		return "", false
	}
	type step struct {
		SkillID     string `json:"skill_id"`
		Note        string `json:"note"`
		Inputs      string `json:"inputs"`
		Credentials string `json:"credentials"`
	}
	var steps []step
	for _, s := range p.Steps {
		if strings.TrimSpace(s.SkillID) == "" {
			continue
		}
		if _, err := svc.store.GetSkill(s.SkillID); err != nil {
			continue // etapa órfã: skill inexistente
		}
		steps = append(steps, step{
			SkillID: s.SkillID, Note: s.Note, Inputs: s.Inputs, Credentials: s.Credentials,
		})
	}
	if len(steps) < 2 {
		return "", false
	}
	payload, _ := json.Marshal(map[string]any{"name": p.Name, "steps": steps})
	projectID := p.ProjectID
	if projectID == "" {
		projectID = scope.ProjectID
	}
	title := p.Title
	if title == "" {
		title = p.Name
	}
	sg := &store.Suggestion{
		ProjectID: projectID,
		Type:      "pipeline",
		Title:     title,
		Payload:   string(payload),
		Evidence:  p.Evidence,
		Origin:    "chat",
	}
	created, err := svc.store.CreateSuggestion(sg)
	if err != nil {
		log.Printf("chat: falha ao inserir pipeline: %v", err)
		return "", false
	}
	svc.publish("suggestion.created", map[string]any{"type": "pipeline", "title": title, "origin": "chat"})
	return created.ID, true
}
