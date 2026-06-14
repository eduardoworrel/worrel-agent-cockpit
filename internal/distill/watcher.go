package distill

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	fsw      *fsnotify.Watcher
	root     string
	debounce time.Duration
	onChange func()
	done     chan struct{}
}

func NewWatcher(root string, debounce time.Duration, onChange func()) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{fsw: fsw, root: root, debounce: debounce, onChange: onChange, done: make(chan struct{})}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			_ = fsw.Add(path)
		}
		return nil
	})
	_ = fsw.Add(root)
	return w, nil
}

func (w *Watcher) Run() {
	var timer *time.Timer
	fire := func() { w.onChange() }
	for {
		select {
		case ev, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			// novas pastas de projeto entram em watch
			if ev.Op&fsnotify.Create != 0 {
				if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
					_ = w.fsw.Add(ev.Name)
				}
			}
			if !strings.HasSuffix(ev.Name, ".jsonl") {
				continue
			}
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(w.debounce, fire)
		case <-w.fsw.Errors:
		case <-w.done:
			return
		}
	}
}

func (w *Watcher) Close() error {
	close(w.done)
	return w.fsw.Close()
}
