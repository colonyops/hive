// Package dirwatch runs a debounced callback when matching files in a
// directory change. It watches the directory, not the files: an atomic save
// replaces a file by rename, which drops a watch on the file itself.
package dirwatch

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/pkg/logutils"
)

const (
	relevantOps   = fsnotify.Create | fsnotify.Write | fsnotify.Rename | fsnotify.Remove
	structuralOps = fsnotify.Create | fsnotify.Rename | fsnotify.Remove
)

type options struct {
	debounce  time.Duration
	component string
	subdirs   bool
}

type Option func(*options)

// WithDebounce sets the quiet period after the last event. Default 250ms.
func WithDebounce(d time.Duration) Option {
	return func(o *options) { o.debounce = d }
}

// WithComponent labels log lines and errors. Default "dirwatch".
func WithComponent(name string) Option {
	return func(o *options) { o.component = name }
}

// WithSubdirectories also watches each immediate subdirectory. A change to
// that set calls back on its own, because a directory moved in with its files
// already inside produces no event for those files.
func WithSubdirectories() Option {
	return func(o *options) { o.subdirs = true }
}

type Watcher struct {
	root     string
	match    func(path string) bool
	onChange func()
	opts     options
	logger   zerolog.Logger
	watcher  *fsnotify.Watcher
	watched  map[string]bool

	started  atomic.Bool
	stopOnce sync.Once
	stop     chan struct{}
	done     chan struct{}
}

// New creates root if it is missing and watches it. Nothing is delivered
// until Start.
func New(root string, match func(path string) bool, onChange func(), logger zerolog.Logger, opts ...Option) (*Watcher, error) {
	o := options{debounce: 250 * time.Millisecond, component: "dirwatch"}
	for _, opt := range opts {
		opt(&o)
	}

	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("%s: create %s: %w", o.component, root, err)
	}
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("%s: start watcher: %w", o.component, err)
	}
	if err := fsw.Add(root); err != nil {
		_ = fsw.Close()
		return nil, fmt.Errorf("%s: watch %s: %w", o.component, root, err)
	}

	w := &Watcher{
		root:     root,
		match:    match,
		onChange: onChange,
		opts:     o,
		logger:   logutils.Component(logger, o.component).With().Str("dir", root).Logger(),
		watcher:  fsw,
		watched:  map[string]bool{},
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	if o.subdirs {
		w.resync()
	}
	return w, nil
}

func (w *Watcher) Start() {
	if w.started.Swap(true) {
		return
	}
	go w.run()
}

// Close waits for an in-flight callback, so it must not be called from one.
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

	debounce := time.NewTimer(w.opts.debounce)
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
			if w.handle(event) {
				debounce.Reset(w.opts.debounce)
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.logger.Warn().Err(err).Msg("watch error")
		case <-debounce.C:
			w.onChange()
		}
	}
}

func (w *Watcher) handle(event fsnotify.Event) bool {
	if event.Op&relevantOps == 0 {
		return false
	}
	changed := w.match(event.Name)
	if w.opts.subdirs && event.Op&structuralOps != 0 && filepath.Dir(event.Name) == w.root && w.resync() {
		changed = true
	}
	return changed
}

// resync reports whether the subdirectory watch set changed.
func (w *Watcher) resync() bool {
	entries, err := os.ReadDir(w.root)
	if err != nil {
		if !os.IsNotExist(err) {
			w.logger.Warn().Err(err).Msg("root read failed")
		}
		changed := len(w.watched) > 0
		for dir := range w.watched {
			_ = w.watcher.Remove(dir)
		}
		clear(w.watched)
		return changed
	}

	current := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			current[filepath.Join(w.root, entry.Name())] = true
		}
	}

	changed := false
	for dir := range w.watched {
		if current[dir] {
			continue
		}
		_ = w.watcher.Remove(dir)
		delete(w.watched, dir)
		changed = true
	}
	for dir := range current {
		if w.watched[dir] {
			continue
		}
		if err := w.watcher.Add(dir); err != nil {
			w.logger.Warn().Err(err).Str("subdir", dir).Msg("subdirectory watch failed")
			continue
		}
		w.watched[dir] = true
		changed = true
	}
	return changed
}
