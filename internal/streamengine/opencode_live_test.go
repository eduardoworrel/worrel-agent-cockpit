//go:build integration

package streamengine

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/eduardoworrel/worrel-agent-cockpit/internal/agui"
)

func TestOpencodeACPLive(t *testing.T) {
	if _, err := exec.LookPath("opencode"); err != nil {
		t.Skip("opencode não instalado")
	}
	done := make(chan struct{}, 8)
	sess, err := opencodeDriver{}.Start(context.Background(), "live1", t.TempDir(),
		Opts{}, func(string) { select { case done <- struct{}{}: default: } }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if err := sess.SendPrompt("responda apenas a palavra ok"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(60 * time.Second)
	for {
		select {
		case <-done:
			if sess.Snapshot().State == agui.StateAwaiting && sess.Snapshot().Message != "" {
				return // sucesso
			}
		case <-deadline:
			t.Fatalf("timeout; último snapshot: %+v", sess.Snapshot())
		}
	}
}
