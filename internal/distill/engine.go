package distill

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// Headless é a fração da interface de adaptador que o motor precisa.
type Headless interface {
	RunHeadless(ctx context.Context, prompt string, opts adapter.HeadlessOpts) (string, error)
}

// AutoApplier é a interface opcional (implementada por *apply.Applier) que o
// engine usa para auto-aplicar sugestões cuja skill tem política auto. Mantida
// como interface para evitar dependência circular distill→apply.
type AutoApplier interface {
	MaybeAutoApply(suggestionID string, dailyCap int) (bool, error)
}

type Engine struct {
	store    *store.Store
	cli      Headless
	bus      *bus.Bus
	auto     AutoApplier
	llmCalls int64
}

// LLMCalls retorna o total de chamadas LLM feitas por este engine.
func (e *Engine) LLMCalls() int64 { return e.llmCalls }

// HeadlessCLI expõe o cliente headless para reuso pela análise retroativa
// (clusterização) sem reimplementar a integração de LLM.
func (e *Engine) HeadlessCLI() Headless { return e.cli }

// SetAutoApplier liga um auto-aplicador (modo automático opt-in, spec §6).
func (e *Engine) SetAutoApplier(a AutoApplier) { e.auto = a }

func New(s *store.Store, cli Headless, b *bus.Bus) *Engine {
	return &Engine{store: s, cli: cli, bus: b}
}

type Result struct {
	Sessions    int `json:"sessions"`
	Created     int `json:"created"`
	Duplicates  int `json:"duplicates"`
	Dropped     int `json:"dropped"`
	ScreenedOut int `json:"screened_out"`
	Proactive   int `json:"proactive"`
	AutoApplied int `json:"auto_applied"`
}

// Sweep roda UMA varredura incremental: todas as sessões com analyzed_at NULL.
// É o caso particular de AnalyzeBatch para o conjunto padrão (spec §v3 forward-compat).
func (e *Engine) Sweep(ctx context.Context) (Result, error) {
	sessions, err := e.store.PendingSweepSessions()
	if err != nil {
		return Result{}, err
	}
	ids := make([]string, len(sessions))
	// map id→projectID for AnalyzeBatch
	projectOf := map[string]string{}
	for i, s := range sessions {
		ids[i] = s.ID
		projectOf[s.ID] = s.ProjectID
	}
	return e.analyzeBatchInternal(ctx, ids, projectOf, AnalyzeOpts{})
}

// AnalyzeBatch analisa um conjunto explícito de sessões dentro de um projeto.
// API pública para reuso pela Fase 8 (análise retroativa orçada por projeto).
func (e *Engine) AnalyzeBatch(ctx context.Context, projectID string, sessionIDs []string) (Result, error) {
	return e.AnalyzeBatchDepth(ctx, projectID, sessionIDs, AnalyzeOpts{})
}

// AnalyzeOpts é o seam de extensão v3 (Fase 8). SuppressSkills descarta candidatos
// skill.* antes da inserção (modo "leve", critério 11), preservando memórias.
type AnalyzeOpts struct {
	SuppressSkills bool
	// Origin marca a origem das sugestões criadas neste lote ("retroativa" na
	// análise retroativa). Vazio mantém o default incremental do store, de modo
	// que a varredura normal permanece inalterada.
	Origin string
	// Headless sobrescreve o cliente headless (adapter) para este lote. Vazio
	// usa o e.cli fixado no boot. Permite que a análise retroativa escolha o
	// provider por run sem refatorar o engine.
	Headless Headless
	// Model sobrescreve o modelo do CLI para este lote (vazio = default do CLI).
	Model string
}

// AnalyzeBatchDepth é AnalyzeBatch com opções. Reusado pela análise retroativa.
func (e *Engine) AnalyzeBatchDepth(ctx context.Context, projectID string, sessionIDs []string, opts AnalyzeOpts) (Result, error) {
	projectOf := map[string]string{}
	for _, id := range sessionIDs {
		projectOf[id] = projectID
	}
	return e.analyzeBatchInternal(ctx, sessionIDs, projectOf, opts)
}

func (e *Engine) analyzeBatchInternal(ctx context.Context, sessionIDs []string, projectOf map[string]string, opts AnalyzeOpts) (Result, error) {
	var res Result
	if len(sessionIDs) == 0 {
		// Sem sessões novas: ainda assim roda o gatilho de saúde proativo
		// (spec §4.3, critério 4) — degradação pode existir independentemente
		// de haver transcrições novas para analisar.
		res.Proactive = e.runHealth()
		e.publish("sweep.finished", map[string]any{"created": 0, "sessions": 0, "proactive": res.Proactive})
		return res, nil
	}
	e.publish("sweep.started", map[string]any{"sessions": len(sessionIDs)})

	// Load sessions from store to get full metadata
	var batch []sessionTranscript
	var sessions []*store.Session
	for _, id := range sessionIDs {
		sess, err := e.store.GetSession(id)
		if err != nil {
			log.Printf("distill: sessão %s não encontrada: %v", id, err)
			continue
		}
		sessions = append(sessions, sess)
		evs := toAdapterEvents(mustEvents(e.store, sess.ID))
		projectID := projectOf[id]
		if projectID == "" {
			projectID = sess.ProjectID
		}
		batch = append(batch, sessionTranscript{SessionID: sess.ID, ProjectID: projectID, Events: evs})
	}
	res.Sessions = len(batch)

	// FASE 1 — screening barato sem LLM (spec §5, critério 5). Se nenhum sinal
	// local justifica a chamada, marcamos as sessões como analisadas e
	// RETORNAMOS SEM tocar em RunHeadless. e.llmCalls permanece intacto.
	if sig := screenSessions(batch); !sig.Pass() {
		res.ScreenedOut = len(batch)
		e.finalizeSessions(sessions)
		res.Proactive = e.runHealth()
		e.publish("sweep.finished", map[string]any{
			"created": 0, "duplicates": 0, "dropped": 0,
			"sessions": res.Sessions, "screened_out": res.ScreenedOut,
		})
		return res, nil
	}

	skills, _ := e.store.ListSkills("")
	pending, _ := e.store.ListSuggestions("", "pending")
	prompt := buildPrompt(e.store.Prompt("skill"), e.store.Prompt("memory"), batch, skills, pending)

	// FASE 2 — confirmação por LLM. Só aqui o contador incrementa.
	// Override de adapter/modelo por lote (análise retroativa escolhe o provider
	// por run); sem override usa o cliente fixado no boot.
	cli := e.cli
	if opts.Headless != nil {
		cli = opts.Headless
	}
	e.llmCalls++
	out, err := cli.RunHeadless(ctx, prompt, adapter.HeadlessOpts{Model: opts.Model})
	if err != nil {
		e.publish("sweep.finished", map[string]any{"error": err.Error()})
		return res, err // sessões NÃO marcadas → re-tentável
	}
	cands, dropped := ParseCandidates(out)
	res.Dropped = dropped

	// project_id de fallback do lote: memória é por projeto (add_memory). Quando
	// todas as sessões do lote pertencem ao mesmo projeto e o modelo omite/erra o
	// project_id, herdamos o projeto do lote.
	batchProject := uniqueBatchProject(batch)

	recent, _ := e.store.RecentlyUpdatedSkills(24)
	for _, c := range cands {
		// Memória pertence a um projeto: ancora no projeto do lote quando o modelo
		// não informou (ou informou) um project_id, garantindo persistência correta.
		if c.Type == "add_memory" {
			if c.ProjectID == "" && batchProject != "" {
				c.ProjectID = batchProject
			}
		}
		// Modo leve (critério 11): descarta candidatos skill.* antes de inserir,
		// preservando apenas memórias/projetos.
		if opts.SuppressSkills && strings.HasPrefix(c.Type, "skill.") {
			res.Dropped++
			continue
		}
		// skill.learned: if it matches a recent skill, convert to skill.variant instead of dropping
		// skill.variant and skill.correction: only drop if a pending suggestion matches (not if they match their target skill)
		if c.Type == "skill.learned" {
			if sk := e.matchingRecentSkill(c, recent); sk != nil {
				c.Type = "skill.variant"
				c.SkillID = sk.ID
				c.ParentSkillIDs = appendUnique(c.ParentSkillIDs, sk.ID)
			}
		}
		// Guarda de integridade: o LLM pode citar um skill_id inexistente
		// (alucinação ou skill de outra base, comum na análise retroativa sobre
		// uma base recém-criada). Inserir referenciaria uma FK morta → erro.
		// Correction sem alvo válido é descartada; nos demais, limpamos o ref.
		if c.SkillID != "" {
			if _, err := e.store.GetSkill(c.SkillID); err != nil {
				if c.Type == "skill.correction" {
					res.Dropped++
					continue
				}
				c.SkillID = ""
				c.ParentSkillIDs = nil
			}
		}
		if e.isDuplicate(c, pending) {
			res.Duplicates++
			continue
		}
		sgID, err := e.insert(c, opts.Origin)
		if err != nil {
			log.Printf("distill: falha ao inserir sugestão: %v", err)
			continue
		}
		res.Created++
		// Modo automático opt-in (spec §6): tenta auto-aplicar; se a política
		// da skill não permitir, a sugestão permanece pending na fila manual.
		if e.auto != nil && sgID != "" {
			cap_ := e.autoDailyCap()
			if applied, aerr := e.auto.MaybeAutoApply(sgID, cap_); aerr == nil && applied {
				res.AutoApplied++
			}
		}
	}
	e.finalizeSessions(sessions)
	res.Proactive = e.runHealth()
	e.publish("sweep.finished", map[string]any{
		"created": res.Created, "duplicates": res.Duplicates,
		"dropped": res.Dropped, "sessions": res.Sessions,
		"proactive": res.Proactive, "auto_applied": res.AutoApplied,
	})
	return res, nil
}

// finalizeSessions marca as sessões como analisadas e abandona usos de skill
// sem desfecho (spec §4.1 fallback na varredura diferida).
func (e *Engine) finalizeSessions(sessions []*store.Session) {
	ids := make([]string, 0, len(sessions))
	for _, sess := range sessions {
		_ = e.store.MarkSessionAnalyzed(sess.ID)
		ids = append(ids, sess.ID)
	}
	if len(ids) > 0 {
		_ = e.store.AbandonOpenUsages(ids...)
	}
}

// runHealth roda o gatilho proativo de saúde (spec §4.3, critério 4) ao fim do
// sweep e devolve quantas correções proativas foram criadas.
func (e *Engine) runHealth() int {
	threshold := e.healthConsecFailures()
	hc := NewHealthChecker(e.store, threshold)
	n, err := hc.CreateProactiveCorrections(e.bus, e.auto, e.autoDailyCap())
	if err != nil {
		log.Printf("distill: health scan: %v", err)
		return 0
	}
	return n
}

func (e *Engine) healthConsecFailures() int {
	return atoiDefault(e.store.GetSetting("health_consec_failures", "2"), 2)
}

func (e *Engine) autoDailyCap() int {
	return atoiDefault(e.store.GetSetting("auto_daily_cap", "3"), 3)
}

// isDuplicate returns true only when an EQUIVALENT PENDING SUGGESTION already exists.
// When a skill.learned candidate matches a recent accepted skill, it is NOT dropped here —
// the caller converts it to skill.variant instead (see analyzeBatchInternal).
func (e *Engine) isDuplicate(c Candidate, pending []*store.Suggestion) bool {
	for _, p := range pending {
		if IsDuplicate(c.Title, p.Title) {
			return true
		}
	}
	return false
}

// matchingRecentSkill returns the first recent skill whose Name is a Jaccard-match for c.Name, or nil.
func (e *Engine) matchingRecentSkill(c Candidate, recent []*store.Skill) *store.Skill {
	for _, sk := range recent {
		if IsDuplicate(c.Name, sk.Name) {
			return sk
		}
	}
	return nil
}

func (e *Engine) insert(c Candidate, origin string) (string, error) {
	sg := &store.Suggestion{
		ProjectID: c.ProjectID, Type: c.Type, Title: c.Title,
		Payload:  candidatePayload(c),
		Evidence: c.Evidence,
		Origin:   origin, // vazio → store aplica default "incremental"
	}
	if c.SkillID != "" {
		sg.SkillID = &c.SkillID
	}
	created, err := e.store.CreateSuggestion(sg)
	if err != nil {
		return "", err
	}
	e.publish("suggestion.created", map[string]any{"type": c.Type, "title": c.Title})
	return created.ID, nil
}

func atoiDefault(s string, def int) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	if s == "" {
		return def
	}
	return n
}

func (e *Engine) publish(typ string, payload any) {
	if e.bus != nil {
		e.bus.Publish(bus.Event{Type: typ, Payload: payload})
	}
}

func candidatePayload(c Candidate) string {
	// add_memory: o applier lê p.Content como o texto da memória. Quando o modelo
	// só preenche description, espelhamos para content para manter compatibilidade.
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

func mustEvents(s *store.Store, sessionID string) []*store.TranscriptEvent {
	evs, _ := s.ListTranscriptEvents(sessionID)
	return evs
}

func toAdapterEvents(evs []*store.TranscriptEvent) []adapter.TranscriptEvent {
	out := make([]adapter.TranscriptEvent, len(evs))
	for i, e := range evs {
		out[i] = adapter.TranscriptEvent{Role: e.Role, Kind: e.Kind, Content: e.Content,
			TokensIn: e.TokensIn, TokensOut: e.TokensOut, CreatedAt: e.CreatedAt}
	}
	return out
}

// uniqueBatchProject devolve o project_id comum a todas as sessões do lote, ou
// "" se o lote abrange múltiplos projetos (caso em que confiamos no project_id
// emitido pelo modelo por candidato).
func uniqueBatchProject(batch []sessionTranscript) string {
	pid := ""
	for _, st := range batch {
		if st.ProjectID == "" {
			continue
		}
		if pid == "" {
			pid = st.ProjectID
		} else if pid != st.ProjectID {
			return ""
		}
	}
	return pid
}

func appendUnique(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			return slice
		}
	}
	return append(slice, s)
}
