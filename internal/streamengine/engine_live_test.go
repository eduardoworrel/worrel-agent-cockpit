package streamengine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/agui"
)

// TestEngineLive dirige um claude DE VERDADE pelo stream-json e prova o ciclo:
// prompt → texto, e uma ferramenta que pede permissão → interrupt → deny.
// Só roda com WORREL_LIVE_CLAUDE=1 (precisa do claude autenticado; gasta tokens).
func TestEngineLive(t *testing.T) {
	if os.Getenv("WORREL_LIVE_CLAUDE") != "1" {
		t.Skip("defina WORREL_LIVE_CLAUDE=1 para rodar (claude real)")
	}
	cwd := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	changes := make(chan struct{}, 64)
	s, err := Start(ctx, "live-1", cwd, Opts{Mode: "default"}, func(string) { select {
		case changes <- struct{}{}:
		default:
		} }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// pede uma ação que exige permissão (Write).
	if err := s.SendPrompt("Crie um arquivo chamado teste.txt com o conteudo OLA usando a ferramenta Write. Nada mais."); err != nil {
		t.Fatal(err)
	}

	// espera o interrupt de permissão aparecer.
	snap := waitFor(t, s, changes, 60*time.Second, func(sn agui.Snapshot) bool {
		return sn.Interrupt != nil
	})
	if snap.Interrupt.Kind != agui.KindPermission {
		t.Fatalf("interrupt.Kind = %q, want permission", snap.Interrupt.Kind)
	}
	t.Logf("permissão pedida: prompt=%q detail=%q", snap.Interrupt.Prompt, snap.Interrupt.Detail)

	// NEGA pelo stream.
	if err := s.Respond(false); err != nil {
		t.Fatal(err)
	}

	// o arquivo NÃO deve existir (gating funcionou).
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(cwd, "teste.txt")); err == nil {
			t.Fatal("arquivo foi criado — o deny não barrou a ferramenta")
		}
		time.Sleep(time.Second)
	}
	t.Log("deny barrou o Write: arquivo não criado ✓")
}

func waitFor(t *testing.T, s *Session, changes <-chan struct{}, timeout time.Duration, ok func(agui.Snapshot) bool) agui.Snapshot {
	t.Helper()
	deadline := time.After(timeout)
	for {
		if sn := s.Snapshot(); ok(sn) {
			return sn
		}
		select {
		case <-changes:
		case <-time.After(time.Second):
		case <-deadline:
			t.Fatalf("timeout esperando condição; estado=%+v", s.Snapshot())
		}
	}
}
