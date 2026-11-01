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
	// maxDeepEntries is how many files and folders each folder directly under the root may
	// have watched below it, which keeps one large tree from using up file descriptors.
	maxDeepEntries = 1024
)

type options struct {
	debounce  time.Duration
	component string
	subdirs   bool
	deep      bool
	skip      func(dir string) bool
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

// WithAllSubdirectories watches subdirectories at every depth, apart from any skip names.
func WithAllSubdirectories(skip func(dir string) bool) Option {
	return func(o *options) {
		o.subdirs = true
		o.deep = true
		o.skip = skip
	}
}

type Watcher struct {
	root     string
	match    func(path string) bool
	onChange func()
	opts     options
	logger   zerolog.Logger
	watcher  *fsnotify.Watcher
	watched  map[string]bool

	warnedDeep atomic.Bool
	started    atomic.Bool
	stopOnce   sync.Once
	stop       chan struct{}
	done       chan struct{}
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
	if w.opts.subdirs && event.Op&structuralOps != 0 && (w.opts.deep || filepath.Dir(event.Name) == w.root) && w.resync() {
		changed = true
	}
	return changed
}

// resync reports whether the subdirectory watch set changed.
func (w *Watcher) resync() bool {
	current, err := w.subdirectories()
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

func (w *Watcher) subdirectories() (map[string]bool, error) {
	current := map[string]bool{}
	if !w.opts.deep {
		entries, err := os.ReadDir(w.root)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				current[filepath.Join(w.root, entry.Name())] = true
			}
		}
		return current, nil
	}
	entries, err := os.ReadDir(w.root)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || (w.opts.skip != nil && w.opts.skip(entry.Name())) {
			continue
		}
		top := filepath.Join(w.root, entry.Name())
		current[top] = true
		budget := maxDeepEntries
		w.addSubdirectories(top, &budget, current)
	}
	return current, nil
}

// addSubdirectories spends budget on entries before watching the folder that holds them,
// because a kqueue watch on a folder opens a descriptor for each file in it. One folder too
// large to watch costs its own edits and leaves the rest of the tree watched.
func (w *Watcher) addSubdirectories(dir string, budget *int, current map[string]bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		w.logger.Debug().Err(err).Str("subdir", dir).Msg("subdirectory read failed")
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() || (w.opts.skip != nil && w.opts.skip(entry.Name())) {
			continue
		}
		sub := filepath.Join(dir, entry.Name())
		held, err := os.ReadDir(sub)
		if err != nil {
			w.logger.Debug().Err(err).Str("subdir", sub).Msg("subdirectory read failed")
			continue
		}
		if len(held) > *budget {
			if !w.warnedDeep.Swap(true) {
				w.logger.Warn().Int("limit", maxDeepEntries).Str("subdir", sub).Msg("too many files to watch, so edits there will not reload")
			}
			continue
		}
		*budget -= len(held)
		current[sub] = true
		w.addSubdirectories(sub, budget, current)
	}
}
