package agui

import "testing"

func TestRequestSummaryPromptIncludesMessage(t *testing.T) {
	p := RequestSummaryPrompt("  Por favor conserte o login  ")
	if !contains(p, "Por favor conserte o login") {
		t.Fatalf("prompt não contém a mensagem: %q", p)
	}
	if !contains(p, "O usuário pediu") {
		t.Fatalf("prompt não orienta o formato esperado: %q", p)
	}
}

func TestParseRequestSummary(t *testing.T) {
	cases := []struct{ in, want string }{
		{"O usuário pediu para consertar o login.", "O usuário pediu para consertar o login."},
		{"  \n\"O usuário pediu X\"\n", "O usuário pediu X"},
		{"Claro!\nO usuário pediu Y", "Claro!"}, // primeira linha não-vazia
		{"", ""},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := ParseRequestSummary(c.in); got != c.want {
			t.Errorf("ParseRequestSummary(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseRequestSummaryTruncates(t *testing.T) {
	long := make([]byte, 300)
	for i := range long {
		long[i] = 'a'
	}
	got := ParseRequestSummary(string(long))
	if len([]rune(got)) > 240 {
		t.Fatalf("não truncou: len=%d", len([]rune(got)))
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
