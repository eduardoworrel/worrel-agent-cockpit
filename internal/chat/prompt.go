package chat

import (
	"fmt"
	"strings"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

const systemInstructions = `Você é o assistente do Chat de Destilação do worrel. Você conversa com o
operador sobre o histórico de sessões de IA dele (transcripts abaixo) e ajuda a
extrair conhecimento reutilizável.

Sua resposta DEVE ter duas partes:
1) Uma resposta em texto natural para o operador (em português).
2) QUANDO E SOMENTE QUANDO você identificar um artefato concreto e acionável,
   acrescente AO FINAL um único array JSON (sem cercas de código) com os
   candidatos. Se não houver artefato, NÃO emita o array.

Tipos de candidato válidos:
- {"type":"skill.learned","name":"...","content":"...","title":"...","evidence":"..."}
- {"type":"skill.variant","skill_id":"<id existente>","name":"...","content":"...","title":"...","evidence":"..."}
- {"type":"add_memory","content":"...","title":"...","evidence":"...","project_id":"..."}
- {"type":"create_project","description":"...","title":"...","evidence":"..."}
- {"type":"pipeline","name":"...","title":"...","steps":[{"skill_id":"<id existente>","note":"...","inputs":"...","credentials":"..."}],"evidence":"...","project_id":"..."}

Regras: use apenas skill_id que apareçam nas evidências; um pipeline precisa de
ao menos 2 etapas com skill_id reais; nunca invente credenciais reais (descreva
qual credencial é necessária, não o valor).`

// buildPrompt monta o prompt completo. O texto retornado JAMAIS contém segredos
// crus: os transcripts já chegam saneados de retrieve().
func buildPrompt(history []*store.ChatMessage, retrieved []retrievedSession, userText string) string {
	var b strings.Builder
	b.WriteString(systemInstructions)
	b.WriteString("\n\n=== CONVERSA ATÉ AGORA ===\n")
	for _, m := range history {
		b.WriteString(strings.ToUpper(m.Role))
		b.WriteString(": ")
		b.WriteString(m.Content)
		b.WriteString("\n")
	}
	b.WriteString("USER: ")
	b.WriteString(userText)
	b.WriteString("\n\n=== TRANSCRIPTS RELEVANTES (segredos já mascarados) ===\n")
	if len(retrieved) == 0 {
		b.WriteString("(nenhuma sessão relevante encontrada no escopo)\n")
	}
	for _, rs := range retrieved {
		b.WriteString(fmt.Sprintf("\n--- sessão %s (cli=%s, projeto=%s) ---\n",
			rs.ref.SessionID, rs.ref.Adapter, rs.ref.ProjectID))
		b.WriteString(rs.transcript)
		b.WriteString("\n")
	}
	b.WriteString("\n=== FIM ===\nResponda agora.")
	return b.String()
}
