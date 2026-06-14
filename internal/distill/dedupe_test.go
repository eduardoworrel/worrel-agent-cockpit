package distill

import "testing"

func TestNormalize(t *testing.T) {
	if got := normalize("Deploy de Produção!"); got != "deploy de producao" {
		t.Fatalf("normalize = %q", got)
	}
}

func TestSimilarityRatio(t *testing.T) {
	cases := []struct {
		a, b string
		dup  bool
	}{
		{"Deploy para staging", "deploy para staging", true},
		{"Deploy de produção", "Deploy para staging", false},
		{"Configurar CI no GitHub", "Configurar CI do GitHub Actions", true}, // overlap alto
		{"Rodar testes", "Escrever documentação", false},
	}
	for _, c := range cases {
		got := IsDuplicate(c.a, c.b)
		if got != c.dup {
			t.Fatalf("IsDuplicate(%q,%q) = %v, want %v (ratio=%.2f)", c.a, c.b, got, c.dup, similarity(c.a, c.b))
		}
	}
}
