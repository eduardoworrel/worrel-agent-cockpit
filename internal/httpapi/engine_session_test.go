package httpapi

import "testing"

// normalizeProvider é a regra de roteamento testável isolada do handler.
func TestNormalizeProvider(t *testing.T) {
	cases := map[string]struct {
		out string
		ok  bool
	}{
		"":            {"claude-code", true}, // default
		"claude-code": {"claude-code", true},
		"opencode":    {"opencode", true},
		"antigravity": {"", false}, // bloqueado: sem protocolo stream
	}
	for in, want := range cases {
		got, ok := normalizeProvider(in)
		if got != want.out || ok != want.ok {
			t.Errorf("normalizeProvider(%q) = (%q,%v), quer (%q,%v)", in, got, ok, want.out, want.ok)
		}
	}
}
