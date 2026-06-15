package distill

import (
	"fmt"
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

const maxTranscriptChars = 12000 // truncamento inteligente por sessão

// buildPrompt monta o prompt da varredura com transcripts, skills e sugestões
// pendentes. skillGuide e memoryGuide são os prompts editáveis (resolvidos via
// store.Prompt) que orientam a extração de skills e de memória.
func buildPrompt(skillGuide, memoryGuide string, batch []sessionTranscript, skills []*store.Skill, pending []*store.Suggestion) string {
	var b strings.Builder
	b.WriteString("Você é um destilador de conhecimento. Analise os transcripts de sessões abaixo ")
	b.WriteString("e extraia conhecimento reutilizável: skills novas, correções/variantes de skills ")
	b.WriteString("existentes e itens de MEMÓRIA do projeto.\n\n")

	b.WriteString(strings.TrimSpace(skillGuide))
	b.WriteString("\n\n")
	b.WriteString(strings.TrimSpace(memoryGuide))
	b.WriteString("\n\n")

	b.WriteString("Skills existentes (id | nome | primeiras linhas):\n")
	for _, sk := range skills {
		b.WriteString(fmt.Sprintf("- %s | %s | %s\n", sk.ID, sk.Name, firstLines(sk.Content, 2)))
	}
	b.WriteString("\nSugestões pendentes (NÃO duplique):\n")
	for _, p := range pending {
		b.WriteString("- " + p.Title + "\n")
	}
	b.WriteString("\nTranscripts:\n")
	for _, st := range batch {
		b.WriteString(fmt.Sprintf("\n=== Sessão %s (project_id=%s) ===\n", st.SessionID, st.ProjectID))
		b.WriteString(truncate(renderEvents(st.Events), maxTranscriptChars))
	}
	b.WriteString("\n\nResponda APENAS com um array JSON. Schema de cada item:\n")
	b.WriteString(`{"type":"skill.learned|skill.correction|skill.variant|add_memory",`)
	b.WriteString(`"title":"...","name":"(skills)","content":"markdown","description":"(add_memory)",`)
	b.WriteString(`"skill_id":"(só correction)","parent_skill_ids":["..."],`)
	b.WriteString(`"evidence":"trecho do transcript + contagem de ocorrências","project_id":"..."}` + "\n")
	b.WriteString("Para add_memory: preencha content (ou description) com a convenção/decisão/fato a lembrar; ")
	b.WriteString("name não é necessário.\n")
	b.WriteString("Sem texto fora do array. Array vazio [] se nada relevante.\n")
	return b.String()
}

func renderEvents(evs []adapter.TranscriptEvent) string {
	var b strings.Builder
	for _, e := range evs {
		b.WriteString(strings.ToUpper(e.Role) + ": " + e.Content + "\n")
	}
	return b.String()
}

func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, " ")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	half := max / 2
	return s[:half] + "\n...[truncado]...\n" + s[len(s)-half:]
}
