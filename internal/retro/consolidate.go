package retro

import (
	"encoding/json"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/apply"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/distill"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// semanticThreshold é o limiar de redundância sobre o conjunto de tokens
// título+corpo (Jaccard). É calibrável: 0.45 funde lições que repetem o mesmo
// vocabulário central (mesmo com títulos divergentes) sem fundir candidatos
// só superficialmente parecidos. Mais baixo => mais agressivo (risco de fundir
// lições distintas); mais alto => sobrevivem mais duplicatas semânticas. Mantemos
// a fusão por título de distill.IsDuplicate (threshold 0.6) como caso particular:
// títulos quase idênticos fundem mesmo sem corpo parecido, preservando os testes
// existentes; o sinal título+corpo cobre o caso de títulos diferentes.
const semanticThreshold = 0.45

// ptEnStopWords são palavras funcionais PT/EN ignoradas no cálculo de similaridade.
var ptEnStopWords = map[string]bool{
	"de": true, "do": true, "da": true, "dos": true, "das": true, "no": true,
	"na": true, "nos": true, "nas": true, "em": true, "o": true, "a": true,
	"os": true, "as": true, "e": true, "ou": true, "para": true, "por": true,
	"com": true, "sem": true, "um": true, "uma": true, "que": true, "se": true,
	"the": true, "of": true, "in": true, "on": true, "at": true, "to": true,
	"and": true, "or": true, "for": true, "is": true, "are": true, "be": true,
	"vs": true, "via": true,
}

// normalizeText: minúsculas, sem acentos, sem pontuação, espaços colapsados.
// Espelha distill.normalize (que é privado) para manter a mesma noção de token.
func normalizeText(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	out, _, err := transform.String(t, s)
	if err != nil {
		out = s
	}
	out = strings.ToLower(out)
	var b strings.Builder
	prevSpace := false
	for _, r := range out {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevSpace = false
		} else if !prevSpace {
			b.WriteRune(' ')
			prevSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

// tokenSet extrai o conjunto de tokens normalizados sem stopwords.
func tokenSet(s string) map[string]struct{} {
	m := map[string]struct{}{}
	for _, w := range strings.Fields(normalizeText(s)) {
		if !ptEnStopWords[w] {
			m[w] = struct{}{}
		}
	}
	return m
}

// jaccard mede a similaridade (interseção/união) entre dois conjuntos de tokens.
func jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for w := range a {
		if _, ok := b[w]; ok {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// suggestionBody extrai o texto de corpo de uma sugestão (payload content/description),
// usado em conjunto com o título para medir redundância semântica.
func suggestionBody(s *store.Suggestion) string {
	var pl map[string]any
	if err := json.Unmarshal([]byte(s.Payload), &pl); err != nil || pl == nil {
		return ""
	}
	var parts []string
	for _, k := range []string{"content", "description"} {
		if v, ok := pl[k].(string); ok && strings.TrimSpace(v) != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " ")
}

// isRedundant decide se duas sugestões (já garantidas do mesmo projeto e tipo)
// são a mesma lição. Funde quando: (a) os títulos são duplicatas pelo critério
// de título existente (distill.IsDuplicate, caso particular preservado), OU
// (b) a similaridade Jaccard sobre o conjunto título+corpo ultrapassa
// semanticThreshold — capturando duplicatas semânticas com títulos diferentes.
func isRedundant(a, b *store.Suggestion) bool {
	if distill.IsDuplicate(a.Title, b.Title) {
		return true
	}
	ta := tokenSet(a.Title + " " + suggestionBody(a))
	tb := tokenSet(b.Title + " " + suggestionBody(b))
	return jaccard(ta, tb) >= semanticThreshold
}

// Consolidator agrupa e funde as sugestões retroativas antes da revisão em lote
// (critérios 7 e 10).
type Consolidator struct {
	store   *store.Store
	applier *apply.Applier // opcional; necessário só para BulkResolve(accept)
}

func NewConsolidator(s *store.Store) *Consolidator { return &Consolidator{store: s} }

func (c *Consolidator) WithApplier(a *apply.Applier) *Consolidator { c.applier = a; return c }

// runProjects devolve os project_ids distintos das sessões da run.
func (c *Consolidator) runProjects(runID string) ([]string, error) {
	rows, err := c.store.DB().Query(`SELECT DISTINCT project_id FROM retro_run_sessions
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

// Consolidate funde sugestões retroativas redundantes por (projeto, tipo) usando
// distill.IsDuplicate sobre os títulos, registrando occurrences e concatenando
// evidências; as demais viram rejected (critério 7).
func (c *Consolidator) Consolidate(runID string) error {
	projects, err := c.runProjects(runID)
	if err != nil {
		return err
	}
	for _, pid := range projects {
		pend, err := c.store.ListSuggestions(pid, "pending")
		if err != nil {
			return err
		}
		// só sugestões de origem retroativa
		var retro []*store.Suggestion
		for _, s := range pend {
			if s.Origin == "retroativa" {
				retro = append(retro, s)
			}
		}
		// agrupa por tipo, depois clusteriza por redundância (título OU
		// título+corpo); nunca funde entre tipos nem entre projetos.
		byType := map[string][]*store.Suggestion{}
		for _, s := range retro {
			byType[s.Type] = append(byType[s.Type], s)
		}
		for _, group := range byType {
			used := make([]bool, len(group))
			for i := range group {
				if used[i] {
					continue
				}
				rep := group[i]
				cluster := []*store.Suggestion{rep}
				used[i] = true
				for j := i + 1; j < len(group); j++ {
					if used[j] {
						continue
					}
					if isRedundant(rep, group[j]) {
						cluster = append(cluster, group[j])
						used[j] = true
					}
				}
				if len(cluster) == 1 {
					// ainda registra occurrences=1 para uniformidade da UI
					c.annotate(rep, 1, nil)
					continue
				}
				// elege como representante o candidato mais completo (maior corpo
				// título+descrição/conteúdo), preservando a melhor descrição/título;
				// os demais viram rejected mas suas evidências são concatenadas.
				best := 0
				for k, m := range cluster {
					if len(m.Title+suggestionBody(m)) > len(cluster[best].Title+suggestionBody(cluster[best])) {
						best = k
					}
				}
				rep = cluster[best]
				var evid []string
				for k, m := range cluster {
					if k == best {
						continue
					}
					evid = append(evid, m.Evidence)
					_ = c.store.ResolveSuggestion(m.ID, "rejected")
				}
				c.annotate(rep, len(cluster), evid)
			}
		}
	}
	return nil
}

// annotate grava occurrences no payload e concatena evidências adicionais.
func (c *Consolidator) annotate(s *store.Suggestion, occ int, extraEvidence []string) {
	var pl map[string]any
	if err := json.Unmarshal([]byte(s.Payload), &pl); err != nil || pl == nil {
		pl = map[string]any{}
	}
	pl["occurrences"] = occ
	b, _ := json.Marshal(pl)
	evidence := s.Evidence
	if len(extraEvidence) > 0 {
		evidence = strings.Join(append([]string{s.Evidence}, extraEvidence...), "\n---\n")
	}
	_ = c.store.UpdateSuggestionContent(s.ID, s.Title, string(b), evidence)
}

// --- Revisão em lote (BatchView / BulkResolve) ---

type TypeGroup struct {
	Type  string       `json:"type"`
	Items []*store.Suggestion `json:"items"`
}

type ProjectGroup struct {
	ProjectID string       `json:"project_id"`
	Groups    []*TypeGroup `json:"groups"`
}

// BatchView monta a árvore projeto → tipo → itens das sugestões retroativas pendentes.
func (c *Consolidator) BatchView(runID string) ([]*ProjectGroup, error) {
	projects, err := c.runProjects(runID)
	if err != nil {
		return nil, err
	}
	var out []*ProjectGroup
	for _, pid := range projects {
		pend, err := c.store.ListSuggestions(pid, "pending")
		if err != nil {
			return nil, err
		}
		typeIdx := map[string]*TypeGroup{}
		pg := &ProjectGroup{ProjectID: pid}
		for _, s := range pend {
			if s.Origin != "retroativa" {
				continue
			}
			g := typeIdx[s.Type]
			if g == nil {
				g = &TypeGroup{Type: s.Type}
				typeIdx[s.Type] = g
				pg.Groups = append(pg.Groups, g)
			}
			g.Items = append(g.Items, s)
		}
		if len(pg.Groups) > 0 {
			out = append(out, pg)
		}
	}
	return out, nil
}

// BulkResolve aplica `action` (accepted|rejected|deferred) a todos os itens de um
// grupo projeto→tipo de origem retroativa (ação em massa, critério 10).
func (c *Consolidator) BulkResolve(runID, projectID, typ, action string) (int, error) {
	pend, err := c.store.ListSuggestions(projectID, "pending")
	if err != nil {
		return 0, err
	}
	n := 0
	for _, s := range pend {
		if s.Origin != "retroativa" || s.Type != typ {
			continue
		}
		if action == "accepted" && c.applier != nil {
			if err := c.applier.Accept(s.ID); err != nil {
				return n, err
			}
		} else {
			if err := c.store.ResolveSuggestion(s.ID, action); err != nil {
				return n, err
			}
		}
		n++
	}
	return n, nil
}
