package metasession

import "testing"

func TestIsWorrelMeta(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"distiller plain", "Você é um destilador de conhecimento. Faça X.", true},
		{"clusterer plain", "Você organiza um histórico de sessões de codificação em projetos.", true},
		{"distiller quoted", `"Você é um destilador de conhecimento. Faça X."`, true},
		{"distiller with caveat", "<local-command-caveat>blah blah caveat</local-command-caveat>Você é um destilador de conhecimento.", true},
		{"real session quoting later", longPrefix + "Você é um destilador de conhecimento.", false},
		{"empty", "", false},
		{"unrelated", "Olá, me ajude a corrigir um bug em Go.", false},
	}
	for _, c := range cases {
		if got := IsWorrelMeta(c.in); got != c.want {
			t.Errorf("%s: IsWorrelMeta()=%v want %v", c.name, got, c.want)
		}
	}
}

// longPrefix empurra a assinatura para fora da janela de detecção (headWindow).
var longPrefix = makeFiller(headWindow + 50)

func makeFiller(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'x'
	}
	return string(b)
}
