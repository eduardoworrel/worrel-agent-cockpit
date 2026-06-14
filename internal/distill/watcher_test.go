package distill

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatcherFiresOnChange(t *testing.T) {
	dir := t.TempDir()
	var fired atomic.Int32
	w, err := NewWatcher(dir, 50*time.Millisecond, func() { fired.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	go w.Run()
	time.Sleep(100 * time.Millisecond)
	os.WriteFile(filepath.Join(dir, "s.jsonl"), []byte("{}\n"), 0o644)
	deadline := time.After(2 * time.Second)
	for fired.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("watcher não disparou")
		default:
			time.Sleep(20 * time.Millisecond)
		}
	}
}
