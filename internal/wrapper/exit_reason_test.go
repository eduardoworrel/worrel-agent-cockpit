package wrapper

import (
	"os/exec"
	"strings"
	"testing"
)

func TestExitReason(t *testing.T) {
	// saída normal sem cauda útil → sem motivo registrado
	if r := exitReason(nil, []byte("  \n ")); r != "" {
		t.Fatalf("exit 0 sem cauda deveria ser vazio, veio %q", r)
	}
	// exit code não-zero é capturado com a cauda do stderr
	err := exec.Command("sh", "-c", "echo boom 1>&2; exit 7").Run()
	r := exitReason(err, []byte("boom"))
	if !strings.Contains(r, "código 7") || !strings.Contains(r, "boom") {
		t.Fatalf("esperava código 7 + boom, veio %q", r)
	}
}

func TestStripANSI(t *testing.T) {
	in := "\x1b[31merro\x1b[0m fatal\x1b[2K"
	if got := stripANSI(in); got != "erro fatal" {
		t.Fatalf("stripANSI = %q", got)
	}
}

func TestLastBytes(t *testing.T) {
	if got := string(lastBytes([]byte("abcdef"), 3)); got != "def" {
		t.Fatalf("lastBytes = %q", got)
	}
	if got := string(lastBytes([]byte("ab"), 5)); got != "ab" {
		t.Fatalf("lastBytes curto = %q", got)
	}
}
