package retro

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/store"
)

func TestDetectMasksAndHashes(t *testing.T) {
	raw := "ghp_0123456789abcdefghij"
	fs := Detect("export GITHUB_TOKEN=" + raw)
	if len(fs) != 1 {
		t.Fatalf("achados = %d, want 1", len(fs))
	}
	f := fs[0]
	if strings.Contains(f.Masked, "0123456789abcdefghij") {
		t.Fatalf("máscara vazou valor cru: %q", f.Masked)
	}
	sum := sha256.Sum256([]byte(raw))
	if f.Hash != hex.EncodeToString(sum[:]) {
		t.Fatalf("hash incorreto: %q", f.Hash)
	}
}

func TestMaskPreservesEnds(t *testing.T) {
	if mask("abc") != "****" {
		t.Fatalf("curto: %q", mask("abc"))
	}
	m := mask("ghp_abcdwxyz")
	if !strings.HasPrefix(m, "ghp_") || !strings.HasSuffix(m, "wxyz") || !strings.Contains(m, "*") {
		t.Fatalf("máscara: %q", m)
	}
}

func TestScanSuppressionAndPayload(t *testing.T) {
	s := newStore(t)
	p, _ := s.CreateProject("App", "")
	sc := NewSecretScan(s)
	raw := "sk-ABCDEFGHIJKLMNOPQRSTUVWX"
	sum := sha256.Sum256([]byte(raw))
	hash := hex.EncodeToString(sum[:])

	// 1ª varredura: emite sugestão mascarada
	n, err := sc.Scan(p.ID, []string{"key=" + raw})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("sugestões = %d, want 1", n)
	}
	sg, _ := s.ListSuggestions(p.ID, "pending")
	var found *store.Suggestion
	for _, x := range sg {
		if x.Type == "secret.detected" {
			found = x
		}
	}
	if found == nil {
		t.Fatal("sugestão secret.detected ausente")
	}
	if strings.Contains(found.Evidence, raw) || strings.Contains(found.Payload, raw) {
		t.Fatal("valor cru vazou em evidência/payload")
	}
	var pl map[string]any
	_ = json.Unmarshal([]byte(found.Payload), &pl)
	if pl["hash"] != hash {
		t.Fatalf("payload hash = %v", pl["hash"])
	}

	// suprimir o hash → próxima varredura não re-sugere
	_ = s.SuppressSecret(hash)
	n2, _ := sc.Scan(p.ID, []string{"key=" + raw})
	if n2 != 0 {
		t.Fatalf("após supressão = %d, want 0", n2)
	}
}
