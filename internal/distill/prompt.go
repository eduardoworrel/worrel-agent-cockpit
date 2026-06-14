package distill

import (
	"fmt"
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/adapter"
	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

const maxTranscriptChars = 12000 // truncamento inteligente por sessão

// buildPrompt monta o prompt da varredura com transcripts, skills e sugestões pendentes.
func buildPrompt(batch []sessionTranscript, skills []*store.Skill, pending []*store.Suggestion) string {
	var b strings.Builder
	b.WriteString("Você é um destilador de conhecimento. Analise os transcripts de sessões abaixo ")
	b.WriteString("e extraia conhecimento reutilizável: skills novas, correções/variantes de skills ")
	b.WriteString("existentes e itens de MEMÓRIA do projeto.\n\n")

	b.WriteString("SKILL vs MEMÓRIA — classifique cada aprendizado:\n")
	b.WriteString("- Só vira SKILL um PROCEDIMENTO acionável: uma forma de EXECUTAR uma tarefa, ")
	b.WriteString("com passos que alguém poderia repetir. Se não há ação a executar, NÃO é skill.\n")
	b.WriteString("- Vira MEMÓRIA (type add_memory) tudo que é conhecimento NÃO-procedimental do projeto: ")
	b.WriteString("convenções, decisões, preferências do usuário e correções recorrentes (fatos/restrições ")
	b.WriteString("que devem ser lembrados, não passos a seguir). Memória é por projeto.\n")
	b.WriteString("Na dúvida entre skill e memória: se descreve COMO fazer algo → skill; se descreve ")
	b.WriteString("ALGO A LEMBRAR (regra/escolha/fato) → add_memory.\n\n")

	b.WriteString("FREQUÊNCIA (priorize o recorrente):\n")
	b.WriteString("- Detecte quando a MESMA tarefa/intenção se repete em VÁRIAS sessões.\n")
	b.WriteString("- Para um padrão recorrente emita UMA ÚNICA sugestão (nunca uma por ocorrência).\n")
	b.WriteString("- Inclua a CONTAGEM de ocorrências na evidence (ex.: \"repetida em N sessões: ...\").\n")
	b.WriteString("- Priorize padrões de ALTA frequência; ignore eventos isolados sem reuso claro.\n\n")

	b.WriteString("WORKFLOWS E TAREFAS PARAMETRIZADAS (não fragmente):\n")
	b.WriteString("- Um PROCEDIMENTO recorrente de MÚLTIPLAS ETAPAS (uma sequência de operações distintas ")
	b.WriteString("que o usuário repete) é UMA ÚNICA skill de workflow — NÃO uma skill por etapa.\n")
	b.WriteString("- Uma TAREFA PARAMETRIZADA recorrente (a mesma operação com um identificador/entrada que ")
	b.WriteString("varia) é UMA ÚNICA skill parametrizada: descreva o parâmetro de entrada e a saída esperada.\n")
	b.WriteString("- No content (markdown) de uma skill de workflow, estruture: etapas NUMERADAS e ordenadas; ")
	b.WriteString("para cada etapa, o que precisa de ENTRADA e o que é DERIVADO/já disponível; ")
	b.WriteString("CREDENCIAIS/RECURSOS externos exigidos por etapa; e VARIANTES/ramificações conhecidas de ")
	b.WriteString("uma etapa. Não invente etapas que não aparecem nos transcripts.\n\n")

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
