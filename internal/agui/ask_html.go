package agui

import (
	"encoding/json"
	"strings"
)

// askHTMLContextTail é quantas linhas recentes do histórico entram no prompt do
// ask_html — contexto o bastante para o HTML fazer sentido sem inflar o custo.
const askHTMLContextTail = 8

// AskHTMLPrompt monta o prompt que pede ao LLM uma apresentação RICA do que a IA
// espera do usuário: um FRAGMENTO HTML (um único <div> com estilo inline, SEM
// <html>/<body>/<script>) e, opcionalmente, um widget de resposta dinâmico. Pedir
// fragmento — e não um documento completo — encurta a saída e acelera a geração.
// O estilo é deliberadamente NÃO fixado. Determinístico (puro) para ser testável.
func AskHTMLPrompt(expects string, context []HistoryLine) string {
	var b strings.Builder
	b.WriteString("A IA espera uma resposta do usuário. Gere uma apresentação RICA, " +
		"DENSA e CLICÁVEL dessa pergunta/decisão para um modal estreito. Responda " +
		"APENAS em JSON, sem texto extra:\n" +
		"{\"html\":\"<um único <div> com estilo inline, em português — FRAGMENTO, " +
		"sem <html>/<head>/<body>/<style>>\"," +
		"\"widget\":null|{\"type\":\"range|options|...\",\"spec\":{...}}}\n" +
		"\n## Regras do html (críticas)\n" +
		"- FRAGMENTO ENXUTO: devolva só o conteúdo (um <div> raiz), nada de boilerplate " +
		"de documento. Quanto mais curto, mais rápido — vá direto ao ponto.\n" +
		"- COMPACTO: aproveite bem o espaço, NÃO desperdice. Padding pequeno (8–12px), " +
		"sem margens grandes, sem hero gigante. O modal é estreito — pense em densidade, " +
		"não em pôster.\n" +
		"- Se há opções de escolha, renderize-as como itens CLICÁVEIS: cada opção é um " +
		"elemento com o atributo data-choice=\"<texto exato a enviar como resposta>\" e " +
		"style com cursor:pointer. NÃO repita as opções fora desses elementos. As opções " +
		"clicáveis SÃO a resposta — não descreva \"clique no botão abaixo\".\n" +
		"- Layout enxuto: prefira lista vertical compacta ou grid de 2 colunas para as " +
		"opções; rótulo curto em negrito + 1 linha de exemplo/detalhe no máximo.\n" +
		"- Estilo livre (cores/tipografia à vontade), mas legível e auto-suficiente: SEM " +
		"<script>, SEM recursos externos, width 100%. Fundo claro.\n" +
		"- widget: use null quando as opções já estão clicáveis no html (caso comum). Só " +
		"preencha para controles que o html não dá conta (ex.: {\"type\":\"range\"," +
		"\"spec\":{\"min\":0,\"max\":100}}).\n" +
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
