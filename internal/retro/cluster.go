package retro

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/bus"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/distill"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// Clusterer agrupa as sessões pendentes da run em projetos propostos. A heurística
// local agrupa por pasta; um único headless refina (funde/divide/nomeia) e associa
// a projetos existentes (critério 6). É a primeira chamada LLM do fluxo.
type Clusterer struct {
	store *store.Store
	cli   distill.Headless
	bus   *bus.Bus
}

func NewClusterer(s *store.Store, cli distill.Headless, b *bus.Bus) *Clusterer {
	return &Clusterer{store: s, cli: cli, bus: b}
}

type proposedCluster struct {
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Dirs              []string `json:"dirs"`
	SessionIDs        []string `json:"session_ids"`
	ExistingProjectID string   `json:"existing_project_id"`
}

func (cl *Clusterer) Propose(ctx context.Context, runID string) error {
	pending, err := cl.store.PendingRunSessions(runID)
	if err != nil {
		return err
	}
	// Heurística local: agrupa por pasta do projeto atribuído na importação.
	byDir := map[string][]string{} // dir -> session ids
	for _, sid := range pending {
		dir := cl.sessionDir(sid)
		byDir[dir] = append(byDir[dir], sid)
	}

	projects, _ := cl.store.ListProjects()
	prompt := cl.buildPrompt(byDir, projects)

	out, err := cl.cli.RunHeadless(ctx, prompt, adapter.HeadlessOpts{})
	if err != nil {
		return err
	}
	_ = cl.store.IncrRunLLMCalls(runID, 1)
	if cl.bus != nil {
		cl.bus.Publish(bus.Event{Type: "retro.cluster.proposed", Payload: map[string]any{"run_id": runID}})
	}

	clusters := parseClusters(out)
	for _, pc := range clusters {
		if strings.TrimSpace(pc.Name) == "" {
			continue
		}
		// Sessões são derivadas LOCALMENTE das pastas que o LLM agrupou — o prompt
		// não carrega IDs (escala O(pastas), não O(sessões)). Fallback: session_ids
		// do LLM, caso ele os tenha devolvido.
		sessionIDs := sessionsForDirs(byDir, pc.Dirs)
		if len(sessionIDs) == 0 {
			sessionIDs = pc.SessionIDs
		}
		dirs, _ := json.Marshal(pc.Dirs)
		sids, _ := json.Marshal(sessionIDs)
		c := &store.RetroCluster{
			RunID: runID, Name: pc.Name, Description: pc.Description,
			Dirs: string(dirs), SessionIDs: string(sids),
		}
		if pc.ExistingProjectID != "" {
			ep := pc.ExistingProjectID
			c.ExistingProjectID = &ep
		}
		if _, err := cl.store.CreateRetroCluster(c); err != nil {
			return err
		}
	}
	return cl.store.SetRetroRunStatus(runID, "clustered")
}

func (cl *Clusterer) sessionDir(sessionID string) string {
	sess, err := cl.store.GetSession(sessionID)
	if err != nil {
		return ""
	}
	if sess.ProjectID == "" {
		return sess.SourceDir
	}
	p, err := cl.store.GetProject(sess.ProjectID)
	if err != nil || len(p.Dirs) == 0 {
		return ""
	}
	return p.Dirs[0]
}

// sessionsForDirs devolve a união das sessões das pastas dadas (mapeamento local
// pós-clusterização — o LLM agrupa pastas, nós atribuímos as sessões).
func sessionsForDirs(byDir map[string][]string, dirs []string) []string {
	var out []string
	for _, d := range dirs {
		out = append(out, byDir[d]...)
	}
	return out
}

// maxSampleTitles é quantos títulos de exemplo por pasta entram no prompt — o
// suficiente para o LLM nomear/agrupar sem despejar todas as sessões.
const maxSampleTitles = 3

// buildPrompt monta o prompt da clusterização a partir de um RESUMO por pasta
// (caminho, contagem e títulos de exemplo) — nunca a lista completa de IDs. Assim
// o tamanho do prompt depende do nº de pastas, não de sessões, e escala para
// janelas grandes (ex.: milhares de sessões em 14+ dias) sem estourar o contexto.
func (cl *Clusterer) buildPrompt(byDir map[string][]string, projects []*store.Project) string {
	var b strings.Builder
	b.WriteString("Você organiza um histórico de sessões de codificação em projetos.\n")
	b.WriteString("Agrupe as PASTAS abaixo em projetos coerentes (funda pastas do mesmo escopo, separe assuntos distintos, nomeie).\n")
	b.WriteString("Use os caminhos das pastas EXATAMENTE como aparecem (campo dirs).\n")
	b.WriteString("Quando reconhecer um projeto existente, preencha existing_project_id.\n\n")
	b.WriteString("Projetos existentes:\n")
	for _, p := range projects {
		b.WriteString(fmt.Sprintf("- id=%s nome=%q dirs=%v\n", p.ID, p.Name, p.Dirs))
	}
	b.WriteString("\nPastas (caminho | nº de sessões | exemplos de título):\n")
	dirs := make([]string, 0, len(byDir))
	for d := range byDir {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	for _, d := range dirs {
		b.WriteString(fmt.Sprintf("- %q | %d sessões | %s\n", d, len(byDir[d]), cl.sampleTitles(byDir[d])))
	}
	b.WriteString("\nResponda APENAS um array JSON de objetos com campos: " +
		"name, description, dirs (array dos caminhos de pasta acima), existing_project_id (string vazia se novo).\n")
	return b.String()
}

// sampleTitles devolve até maxSampleTitles títulos de sessões da pasta, para dar
// contexto ao LLM sem listar todas as sessões.
func (cl *Clusterer) sampleTitles(sids []string) string {
	var titles []string
	for _, sid := range sids {
		if len(titles) >= maxSampleTitles {
			break
		}
		sess, err := cl.store.GetSession(sid)
		if err != nil || strings.TrimSpace(sess.Title) == "" {
			continue
		}
		titles = append(titles, strconv.Quote(sess.Title))
	}
	return strings.Join(titles, "; ")
}

func parseClusters(raw string) []proposedCluster {
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, "["); i >= 0 {
		if j := strings.LastIndex(s, "]"); j > i {
			s = s[i : j+1]
		}
	}
	var items []proposedCluster
	if err := json.Unmarshal([]byte(s), &items); err != nil {
		return nil
	}
	return items
}
