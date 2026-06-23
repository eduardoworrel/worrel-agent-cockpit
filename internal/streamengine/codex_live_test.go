//go:build integration

package streamengine

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/agui"
)

func TestCodexAppServerLive(t *testing.T) {
	if _, err := exec.LookPath("codex"); err != nil {
		t.Skip("codex não instalado")
	}
	done := make(chan struct{}, 8)
	onChange := func(string) { select { case done <- struct{}{}: default: } }
	sess, err := codexDriver{}.Start(context.Background(), "live1", t.TempDir(), Opts{}, onChange, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if err := sess.SendPrompt("responda apenas a palavra ok"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(90 * time.Second)
	for {
		select {
		case <-done:
			snap := sess.Snapshot()
			if snap.State == agui.StateAwaiting && snap.Message != "" {
				return
			}
		case <-deadline:
			t.Fatalf("timeout; snapshot: %+v", sess.Snapshot())
		}
	}
}
