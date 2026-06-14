package chat

import (
	"strings"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/distill"
)

func TestRedactCatchesPrefixedApiKey(t *testing.T) {
	in := "API key do Guara: gk_live_HVc9GeTkv7jOATibiEZc3SfY21h1ICKd fim"
	out, n := redactSecrets(in)
	if n == 0 {
		t.Fatal("não detectou a chave gk_live_…")
	}
	if strings.Contains(out, "gk_live_HVc9GeTkv7jOATibiEZc3SfY21h1ICKd") {
		t.Fatalf("chave crua vazou após redação: %q", out)
	}
}

func TestSanitizeJSONFixesRawNewlines(t *testing.T) {
	// Array com quebra de linha CRUA dentro de uma string → JSON inválido.
	raw := "Eis os candidatos:\n[{\"type\":\"add_memory\",\"title\":\"t\",\"content\":\"linha1\nlinha2\",\"evidence\":\"e\"}]"
	if cands, _ := distill.ParseCandidates(raw); len(cands) != 0 {
		t.Fatalf("esperava parse falhar no JSON cru, veio %d", len(cands))
	}
	fixed := sanitizeCandidateJSON(raw)
	cands, _ := distill.ParseCandidates(fixed)
	if len(cands) != 1 || cands[0].Type != "add_memory" {
		t.Fatalf("após saneamento esperava 1 add_memory, veio %+v", cands)
	}
	if !strings.Contains(extractText(fixed), "Eis os candidatos:") || strings.Contains(extractText(fixed), "[{") {
		t.Fatalf("extractText deveria manter a prosa e remover o array: %q", extractText(fixed))
	}
}
