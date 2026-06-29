package agui

import "strings"

// RequestSummaryPrompt monta o prompt que pede ao LLM para CONDENSAR fielmente o
// último pedido do usuário — sem inferir intenção, sem adicionar passos. É o que
// preenche o bloco "Seu pedido" do modal (o user_message cru costuma ser longo
// demais ou se perder no fluxo). Determinístico (puro) para ser testável.
func RequestSummaryPrompt(userMessage string) string {
	msg := strings.TrimSpace(userMessage)
	if len(msg) > 2000 {
		msg = msg[:1999] + "…"
	}
	var b strings.Builder
	b.WriteString("Condense FIELMENTE o pedido do usuário abaixo em UMA frase curta, " +
		"em português, começando por \"O usuário pediu \". Não infira intenção, não " +
		"adicione passos, não opine — só resuma o que está escrito. Responda APENAS " +
		"a frase, sem aspas e sem texto extra.\n\n" +
		"## Pedido do usuário\n" + msg + "\n")
	return b.String()
}

// ParseRequestSummary limpa a saída do LLM (uma frase). Tolerante a aspas/cercas/
// linhas extras. Em saída vazia devolve "" — a borda cai no fallback (user_message).
func ParseRequestSummary(out string) string {
	s := strings.TrimSpace(out)
	// pega só a primeira linha não-vazia (defende contra preâmbulo do modelo).
	for _, line := range strings.Split(s, "\n") {
		if l := strings.TrimSpace(line); l != "" {
			s = l
			break
		}
	}
	s = strings.Trim(s, "\"'`")
	s = strings.TrimSpace(s)
	if len(s) > 240 {
		s = s[:239] + "…"
	}
	return s
}
