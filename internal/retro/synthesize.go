package retro

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/distill"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// Synthesizer é o passo de SÍNTESE por projeto (segunda ordem): roda DEPOIS da
// destilação+consolidação e olha as skills/memórias JÁ destiladas de um projeto
// para reconhecer quando várias são ETAPAS de um mesmo workflow recorrente —
// propondo UMA skill de workflow unificada que as encadeia. A destilação por lote
// só enxerga 5 sessões por vez, então etapas de um pipe executadas em sessões/dias
// diferentes saem fragmentadas; esta passada costura sobre o texto já destilado
// (barato: 1 chamada por projeto, não sobre os transcripts crus).
type Synthesizer struct {
	store *store.Store
	cli   distill.Headless
	bus   *bus.Bus
}

func NewSynthesizer(s *store.Store, cli distill.Headless, b *bus.Bus) *Synthesizer {
	return &Synthesizer{store: s, cli: cli, bus: b}
}

// maxSynthItems limita quantos itens entram no prompt, para não estourar o
// contexto em projetos muito ativos. Todas as skills entram primeiro; memórias
// preenchem o restante do orçamento como contexto.
const maxSynthItems = 40

// Synthesize roda a síntese para todos os projetos da run. Erros por projeto são
// tolerados (best-effort): a síntese é um reforço, não deve derrubar o run.
func (sy *Synthesizer) Synthesize(ctx context.Context, runID string) error {
	pids, err := sy.runProjects(runID)
	if err != nil {
		return err
	}
	for _, pid := range pids {
		if err := sy.synthesizeProject(ctx, runID, pid); err != nil {
			// best-effort: registra via bus e segue
			if sy.bus != nil {
				sy.bus.Publish(bus.Event{Type: "retro.synthesize.error", Payload: map[string]any{
					"run_id": runID, "project_id": pid, "error": err.Error()}})
			}
		}
	}
	return nil
}

func (sy *Synthesizer) synthesizeProject(ctx context.Context, runID, projectID string) error {
	pending, err := sy.store.ListSuggestions(projectID, "pending")
	if err != nil {
		return err
	}
	// As ETAPAS de um workflow são as skills (procedimentos); memórias são
	// convenções/fatos de apoio. Num projeto ativo as memórias são MUITAS e, se
	// misturadas num teto por recência, afogam as poucas skills — então a síntese
	// nunca vê o pipe inteiro. Por isso incluímos TODAS as skills e usamos as
	// memórias só como preenchimento de contexto até o teto.
	var skills, mems []*store.Suggestion
	existingTitles := map[string]bool{}
	for _, s := range pending {
		existingTitles[strings.ToLower(strings.TrimSpace(s.Title))] = true
		switch {
		case s.Type == "skill.learned" || s.Type == "skill.variant":
			skills = append(skills, s)
		case s.Type == "add_memory":
			mems = append(mems, s)
		}
	}
	// Um workflow precisa de ≥2 etapas (skills) para valer a costura.
	if len(skills) < 2 {
		return nil
	}
	items := skills
	for _, m := range mems {
		if len(items) >= maxSynthItems {
			break
		}
		items = append(items, m)
	}

	prompt := buildSynthPrompt(items, projectID)
	out, err := sy.cli.RunHeadless(ctx, prompt, adapter.HeadlessOpts{})
	if err != nil {
		return err
	}
	_ = sy.store.IncrRunLLMCalls(runID, 1)

	cands, _ := distill.ParseCandidates(out)
	created := 0
	for _, c := range cands {
		if c.Type != "skill.learned" {
			continue // a síntese só propõe skill de workflow
		}
		if strings.TrimSpace(c.Title) == "" || existingTitles[strings.ToLower(strings.TrimSpace(c.Title))] {
			continue // idempotência leve: não duplica título já pendente
		}
		_, err := sy.store.CreateSuggestion(&store.Suggestion{
			ProjectID: projectID,
			Type:      "skill.learned",
			Title:     c.Title,
			Payload:   synthPayload(c),
			Evidence:  c.Evidence,
			Origin:    "retroativa",
		})
		if err != nil {
			return err
		}
		existingTitles[strings.ToLower(strings.TrimSpace(c.Title))] = true
		created++
	}
	if created > 0 && sy.bus != nil {
		sy.bus.Publish(bus.Event{Type: "retro.synthesize.proposed", Payload: map[string]any{
			"run_id": runID, "project_id": projectID, "created": created}})
	}
	return nil
}

// runProjects devolve os projetos com sessões nesta run.
func (sy *Synthesizer) runProjects(runID string) ([]string, error) {
	rows, err := sy.store.DB().Query(`SELECT DISTINCT project_id FROM retro_run_sessions
		WHERE run_id=? AND project_id IS NOT NULL`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var pid string
		if err := rows.Scan(&pid); err != nil {
			return nil, err
		}
		out = append(out, pid)
	}
	return out, rows.Err()
}

// buildSynthPrompt monta o prompt de síntese a partir dos itens JÁ destilados
// (não dos transcripts). Totalmente genérico — sem domínios nem exemplos.
func buildSynthPrompt(items []*store.Suggestion, projectID string) string {
	var b strings.Builder
	b.WriteString("Você recebe as SKILLS e MEMÓRIAS já destiladas de UM projeto. ")
	b.WriteString("Algumas podem ser ETAPAS de um mesmo PROCEDIMENTO recorrente de múltiplas etapas que o usuário repete.\n\n")
	b.WriteString("Tarefa: identifique se um SUBCONJUNTO destes itens forma um workflow recorrente de várias etapas. ")
	b.WriteString("Se sim, proponha UMA ÚNICA skill de workflow (type skill.learned) que:\n")
	b.WriteString("- encadeie as etapas em ORDEM;\n")
	b.WriteString("- para cada etapa, indique a ENTRADA necessária e o que é DERIVADO/já disponível;\n")
	b.WriteString("- liste CREDENCIAIS/RECURSOS externos exigidos por etapa;\n")
	b.WriteString("- registre VARIANTES/ramificações conhecidas de uma etapa;\n")
	b.WriteString("- no campo evidence, cite os TÍTULOS dos itens que ela unifica.\n")
	b.WriteString("Não invente etapas que não estejam nos itens. Pode propor mais de um workflow se houver mais de um fluxo distinto. ")
	b.WriteString("Se NÃO houver workflow multi-etapa claro, responda [].\n\n")
	b.WriteString("Itens do projeto (tipo | título | resumo):\n")
	for _, s := range items {
		b.WriteString(fmt.Sprintf("- %s | %s | %s\n", s.Type, s.Title, synthBody(s.Payload)))
	}
	b.WriteString("\nResponda APENAS um array JSON. Cada item: " +
		`{"type":"skill.learned","title":"...","name":"...","content":"markdown com etapas numeradas",` +
		`"evidence":"unifica: <títulos dos itens>","project_id":"` + projectID + `"}` + "\n")
	b.WriteString("Sem texto fora do array.\n")
	return b.String()
}

// synthBody extrai um resumo curto do payload de uma sugestão (content/description).
func synthBody(payload string) string {
	var p struct {
		Content     string `json:"content"`
		Description string `json:"description"`
	}
	_ = json.Unmarshal([]byte(payload), &p)
	body := p.Content
	if body == "" {
		body = p.Description
	}
	body = strings.ReplaceAll(body, "\n", " ")
	if len(body) > 200 {
		body = body[:200] + "…"
	}
	return body
}

// synthPayload monta o payload de skill compatível com o applier (mesmo shape de
// candidatePayload do distill: name/content/description/parent_skill_ids).
func synthPayload(c distill.Candidate) string {
	content := c.Content
	if content == "" {
		content = c.Description
	}
	b, _ := json.Marshal(map[string]any{
		"name": c.Name, "content": content, "description": c.Description,
		"parent_skill_ids": c.ParentSkillIDs,
	})
	return string(b)
}
