package chat

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

// Orçamento de recuperação de contexto.
const (
	maxSessions     = 12    // nº máximo de sessões injetadas no prompt
	maxCharsPerSess = 12000 // nº máximo de chars por transcript de sessão
)

// Scope delimita o universo de sessões recuperadas como contexto. Campos vazios =
// sem filtro (busca global).
type Scope struct {
	ProjectID  string   `json:"project_id"`
	Cluster    string   `json:"cluster"`
	WindowDays int      `json:"window_days"`
	CLIs       []string `json:"clis"`
}

func parseScope(raw string) Scope {
	var sc Scope
	if strings.TrimSpace(raw) == "" {
		return sc
	}
	_ = json.Unmarshal([]byte(raw), &sc)
	return sc
}

func (sc Scope) matchesCLI(adapter string) bool {
	if len(sc.CLIs) == 0 {
		return true
	}
	for _, c := range sc.CLIs {
		if strings.EqualFold(c, adapter) {
			return true
		}
	}
	return false
}

// SessionRef descreve uma sessão escolhida como fonte do contexto (reportada ao
// chamador e persistida no campo sources da mensagem do assistant).
type SessionRef struct {
	SessionID string `json:"session_id"`
	ProjectID string `json:"project_id"`
	Adapter   string `json:"adapter"`
	Title     string `json:"title"`
	Score     int    `json:"score"`
}

// retrievedSession agrega o ref + o transcript já saneado para montar o prompt.
type retrievedSession struct {
	ref        SessionRef
	transcript string // já redigido (sem segredos crus)
}

// retrieve busca sessões relevantes ao texto do usuário dentro do escopo do
// thread, pontuando por relevância (overlap de termos) + recência, e respeitando
// o orçamento. O transcript de cada sessão é truncado e SANEADO (segredos
// mascarados) antes de retornar.
func (svc *Service) retrieve(sc Scope, userText string) ([]retrievedSession, error) {
	sessions, err := svc.store.ListSessions(sc.ProjectID)
	if err != nil {
		return nil, err
	}
	terms := tokenize(userText)

	type scored struct {
		sess  *store.Session
		body  string
		score int
	}
	var cands []scored
	// ListSessions já vem por started_at DESC; índice baixo = mais recente.
	for i, sess := range sessions {
		if !sc.matchesCLI(sess.Adapter) {
			continue
		}
		evs, err := svc.store.ListTranscriptEvents(sess.ID)
		if err != nil {
			return nil, err
		}
		if len(evs) == 0 {
			continue
		}
		var b strings.Builder
		for _, e := range evs {
			b.WriteString(e.Role)
			b.WriteString(": ")
			b.WriteString(e.Content)
			b.WriteString("\n")
		}
		body := b.String()
		score := relevanceScore(body, terms)
		// Bônus de recência: as 5 mais recentes ganham um empurrão decrescente.
		if i < 5 {
			score += (5 - i)
		}
		if score == 0 {
			continue
		}
		cands = append(cands, scored{sess: sess, body: body, score: score})
	}

	sort.SliceStable(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
	if len(cands) > maxSessions {
		cands = cands[:maxSessions]
	}

	out := make([]retrievedSession, 0, len(cands))
	for _, c := range cands {
		body := c.body
		if len(body) > maxCharsPerSess {
			body = body[:maxCharsPerSess]
		}
		body, _ = redactSecrets(body)
		out = append(out, retrievedSession{
			ref: SessionRef{
				SessionID: c.sess.ID,
				ProjectID: c.sess.ProjectID,
				Adapter:   c.sess.Adapter,
				Title:     c.sess.Title,
				Score:     c.score,
			},
			transcript: body,
		})
	}
	return out, nil
}

func tokenize(s string) map[string]bool {
	out := map[string]bool{}
	for _, f := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}) {
		if len(f) >= 3 {
			out[f] = true
		}
	}
	return out
}

func relevanceScore(body string, terms map[string]bool) int {
	if len(terms) == 0 {
		return 1 // sem termos úteis: tudo é igualmente (pouco) relevante
	}
	low := strings.ToLower(body)
	score := 0
	for t := range terms {
		if strings.Contains(low, t) {
			score += 3
		}
	}
	return score
}
