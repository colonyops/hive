// Package dirwatch runs a callback when matching files in one flat directory
// change on disk.
//
// It watches the directory rather than the files: editors and atomic writers
// replace a file by rename, which silently drops a watch registered on the
// file itself, and a directory watch also sees a file that does not exist yet.
// fsnotify is not recursive, so nothing below the directory is seen.
package dirwatch

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/pkg/logutils"
)

const relevantOps = fsnotify.Create | fsnotify.Write | fsnotify.Rename | fsnotify.Remove

type Config struct {
	// Dir is created if missing, then watched.
	Dir string
	// Match reports whether an event path inside Dir is worth a callback.
	Match func(path string) bool
	// Debounce coalesces a burst of events (an editor's write+rename+chmod, a
	// git checkout) into one callback, Debounce after the last of them.
	Debounce time.Duration
	OnChange func()
	// Component labels the watcher's log lines.
	Component string
}

type Watcher struct {
	cfg     Config
	logger  zerolog.Logger
	watcher *fsnotify.Watcher

	started  atomic.Bool
	stopOnce sync.Once
	stop     chan struct{}
	done     chan struct{}
}

// New creates and watches cfg.Dir. Events are not delivered until Start.
func New(cfg Config, logger zerolog.Logger) (*Watcher, error) {
	if err := os.MkdirAll(cfg.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("%s: create %s: %w", cfg.Component, cfg.Dir, err)
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("%s: start watcher: %w", cfg.Component, err)
	}
	if err := watcher.Add(cfg.Dir); err != nil {
		_ = watcher.Close()
		return nil, fmt.Errorf("%s: watch %s: %w", cfg.Component, cfg.Dir, err)
	}
	return &Watcher{
		cfg:     cfg,
		logger:  logutils.Component(logger, cfg.Component).With().Str("dir", cfg.Dir).Logger(),
		watcher: watcher,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}, nil
}

// Start runs the watch loop in a goroutine until Close.
func (w *Watcher) Start() {
	if w.started.Swap(true) {
		return
	}
	go w.run()
}

// Close stops the watcher and waits for an in-flight callback to return. It
// is safe to call more than once, but not from inside OnChange.
func (w *Watcher) Close() {
	w.stopOnce.Do(func() {
		close(w.stop)
		if err := w.watcher.Close(); err != nil {
			w.logger.Debug().Err(err).Msg("watcher close failed")
		}
		if w.started.Load() {
			<-w.done
		}
	})
}

func (w *Watcher) run() {
	defer close(w.done)

	debounce := time.NewTimer(w.cfg.Debounce)
	debounce.Stop()
	defer debounce.Stop()

	for {
		select {
		case <-w.stop:
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Op&relevantOps == 0 || !w.cfg.Match(event.Name) {
				continue
			}
			debounce.Reset(w.cfg.Debounce)
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.logger.Warn().Err(err).Msg("watch error")
		case <-debounce.C:
			w.cfg.OnChange()
		}
	}
}
