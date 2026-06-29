package agui

import (
	"encoding/json"
	"strings"
)

// askHTMLContextTail é quantas linhas recentes do histórico entram no prompt do
// ask_html — contexto o bastante para o HTML fazer sentido sem inflar o custo.
const askHTMLContextTail = 8

// AskHTMLPrompt monta o prompt que pede ao LLM uma apresentação RICA do que a IA
// espera do usuário: um documento HTML completo (estilo inline, SEM <script>) e,
// opcionalmente, um widget de resposta dinâmico. O estilo é deliberadamente NÃO
// fixado — queremos observar como varia. Determinístico (puro) para ser testável.
func AskHTMLPrompt(expects string, context []HistoryLine) string {
	var b strings.Builder
	b.WriteString("A IA espera uma resposta do usuário. Gere uma apresentação RICA e " +
		"LEGÍVEL dessa pergunta/decisão para um modal. Responda APENAS em JSON, sem " +
		"texto extra:\n" +
		"{\"html\":\"<documento HTML completo, com estilo inline, em português; SEM " +
		"<script>, SEM recursos externos>\",\"widget\":null|{\"type\":\"range|options|...\"," +
		"\"spec\":{...}}}\n" +
		"- html: HTML autossuficiente (será renderizado num iframe isolado). Estilo livre.\n" +
		"- widget: opcional. Descreva COMO pedir o dado se um controle ajudar (ex.: " +
		"{\"type\":\"range\",\"spec\":{\"min\":0,\"max\":100}} ou {\"type\":\"options\"," +
		"\"spec\":{\"options\":[\"a\",\"b\"]}}). Use null se um campo de texto basta.\n" +
		"Não invente conteúdo — baseie-se só no que a IA espera e no contexto.\n\n")
	if len(context) > 0 {
		tail := context
		if len(tail) > askHTMLContextTail {
			tail = tail[len(tail)-askHTMLContextTail:]
		}
		b.WriteString("## Contexto recente\n")
		for _, h := range tail {
			b.WriteString("[" + h.Role + "] " + h.Text + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("## O que a IA espera\n" + strings.TrimSpace(expects) + "\n")
	return b.String()
}

// AskHTML é o resultado estruturado da engine ask_html.
type AskHTML struct {
	HTML   string
	Widget *ResponseWidget
}

// ParseAskHTML extrai o JSON {html, widget} da saída do LLM (tolerante a cercas/
// lixo ao redor). Em falha devolve AskHTML zerado (HTML="" → a borda cai no
// fallback markdown). O HTML nunca bloqueia o modal.
func ParseAskHTML(out string) AskHTML {
	start := strings.IndexByte(out, '{')
	end := strings.LastIndexByte(out, '}')
	if start < 0 || end <= start {
		return AskHTML{}
	}
	var raw struct {
		HTML   string          `json:"html"`
		Widget *ResponseWidget `json:"widget"`
	}
	if json.Unmarshal([]byte(out[start:end+1]), &raw) != nil {
		return AskHTML{}
	}
	res := AskHTML{HTML: strings.TrimSpace(raw.HTML)}
	if raw.Widget != nil && strings.TrimSpace(raw.Widget.Type) != "" {
		res.Widget = raw.Widget
	}
	return res
}
