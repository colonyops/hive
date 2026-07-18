package feed

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
)

// watchDebounce coalesces editor save bursts (write + rename + chmod) into
// one reload.
const watchDebounce = 250 * time.Millisecond

// ConfigWatcher invokes onChange when the profiles config file changes on
// disk, so edits made outside the app apply live. It watches the parent
// directory rather than the file: editors and atomic writers replace the
// file by rename, which silently drops a watch registered on the file
// itself — and lets the watch work before the file first exists.
type ConfigWatcher struct {
	path     string
	onChange func()
	logger   zerolog.Logger
	watcher  *fsnotify.Watcher

	stopOnce sync.Once
	stop     chan struct{}
}

func NewConfigWatcher(path string, onChange func(), logger zerolog.Logger) (*ConfigWatcher, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("feed: create config dir: %w", err)
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("feed: start config watcher: %w", err)
	}
	if err := watcher.Add(dir); err != nil {
		_ = watcher.Close()
		return nil, fmt.Errorf("feed: watch %s: %w", dir, err)
	}
	return &ConfigWatcher{
		path:     path,
		onChange: onChange,
		logger:   logger,
		watcher:  watcher,
		stop:     make(chan struct{}),
	}, nil
}

// Start runs the watch loop in a goroutine until Close.
func (w *ConfigWatcher) Start() {
	go w.run()
}

func (w *ConfigWatcher) Close() {
	w.stopOnce.Do(func() {
		close(w.stop)
		if err := w.watcher.Close(); err != nil {
			w.logger.Debug().Err(err).Msg("config watcher close failed")
		}
	})
}

func (w *ConfigWatcher) run() {
	var debounce <-chan time.Time
	for {
		select {
		case <-w.stop:
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			// Only the watched directory feeds this channel, so basename
			// equality identifies the config file across write/rename paths.
			if filepath.Base(event.Name) != filepath.Base(w.path) {
				continue
			}
			if !event.Op.Has(fsnotify.Create) && !event.Op.Has(fsnotify.Write) &&
				!event.Op.Has(fsnotify.Rename) && !event.Op.Has(fsnotify.Remove) {
				continue
			}
			debounce = time.After(watchDebounce)
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.logger.Warn().Err(err).Msg("config watch error")
		case <-debounce:
			debounce = nil
			w.onChange()
		}
	}
}
