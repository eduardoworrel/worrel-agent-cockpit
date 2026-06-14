package opencode

import "testing"

func TestParseOpencodeModelsFixture(t *testing.T) {
	// Fixture no formato real de `opencode models` (provider/model por linha),
	// com linha vazia e duplicata para exercitar trim/dedup.
	fixture := []byte("opencode/big-pickle\nopencode-go/glm-5\n\nminimax-coding-plan/MiniMax-M2\nopencode/big-pickle\n")
	got := parseOpencodeModels(fixture)
	want := []string{"opencode/big-pickle", "opencode-go/glm-5", "minimax-coding-plan/MiniMax-M2"}
	if len(got) != len(want) {
		t.Fatalf("parsed %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parsed[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
