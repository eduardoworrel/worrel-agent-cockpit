package agui

import "testing"

func TestAskHTMLPromptIncludesExpectsAndContext(t *testing.T) {
	p := AskHTMLPrompt("Posso deletar o arquivo X?", []HistoryLine{{Role: "ai", Text: "olá"}})
	if !contains(p, "Posso deletar o arquivo X?") {
		t.Fatalf("prompt não contém o expects: %q", p)
	}
	if !contains(p, "olá") {
		t.Fatalf("prompt não contém o contexto: %q", p)
	}
}

func TestParseAskHTMLHappy(t *testing.T) {
	out := "lixo antes {\"html\":\"<h1>Oi</h1>\",\"widget\":{\"type\":\"range\",\"spec\":{\"min\":0,\"max\":10}}} lixo depois"
	res := ParseAskHTML(out)
	if res.HTML != "<h1>Oi</h1>" {
		t.Fatalf("HTML = %q", res.HTML)
	}
	if res.Widget == nil || res.Widget.Type != "range" {
		t.Fatalf("widget inesperado: %+v", res.Widget)
	}
}

func TestParseAskHTMLNullWidget(t *testing.T) {
	res := ParseAskHTML(`{"html":"<p>x</p>","widget":null}`)
	if res.HTML != "<p>x</p>" || res.Widget != nil {
		t.Fatalf("res = %+v", res)
	}
}

func TestParseAskHTMLMalformedFallsBack(t *testing.T) {
	for _, in := range []string{"", "não é json", "{quebrado", `{"widget":{"type":""}}`} {
		res := ParseAskHTML(in)
		if res.HTML != "" {
			t.Errorf("ParseAskHTML(%q).HTML = %q, want vazio", in, res.HTML)
		}
		if res.Widget != nil {
			t.Errorf("ParseAskHTML(%q).Widget = %+v, want nil", in, res.Widget)
		}
	}
}
