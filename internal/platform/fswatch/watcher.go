// Package fswatch watches a directory tree and reports which directories
// changed. fsnotify has no recursive watch, so the watcher holds one watch per
// directory and keeps that set in step with the tree. It reports directories
// rather than files because its consumer re-reads a changed directory instead
// of applying event deltas.
//
// Symbolic links below the root are not followed: a linked directory is
// neither walked nor watched. The root itself may be a link.
package fswatch

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/pkg/logutils"
)

const relevantOps = fsnotify.Create | fsnotify.Write | fsnotify.Remove | fsnotify.Rename

type options struct {
	debounce time.Duration
	skipDir  func(name string) bool
}

type Option func(*options)

// WithDebounce sets the window that collects events into one Change. The
// window opens at the first event after the previous one closed and closes d
// later, so a continuous writer still yields a Change every d. Default 100ms.
func WithDebounce(d time.Duration) Option {
	return func(o *options) { o.debounce = d }
}

// WithSkipDir replaces the rule that leaves a directory unwatched. fn sees the
// base name of every directory below the root. The default skips names
// starting with ".", which keeps .git, .obsidian, and .hive out of the set.
func WithSkipDir(fn func(name string) bool) Option {
	return func(o *options) { o.skipDir = fn }
}

func defaultSkipDir(name string) bool { return strings.HasPrefix(name, ".") }

// Change lists the absolute directories whose listing or files changed since
// the previous Change, sorted. A created directory is listed itself as well
// as through its parent: files written into it before its watch existed
// produced no events, so the consumer has to read it.
type Change struct{ Dirs []string }

// Watcher delivers one Change per debounce window on an unbuffered channel.
// It never blocks on its consumer: windows that close while the consumer is
// busy merge into the Change it receives next.
type Watcher struct {
	logger  zerolog.Logger
	root    string
	opts    options
	fsw     *fsnotify.Watcher
	changes chan Change

	// dirs and pending belong to the pump goroutine after New returns.
	dirs    map[string]struct{}
	pending map[string]struct{}

	closeOnce sync.Once
	stop      chan struct{}
	done      chan struct{}
}

// New walks root, adds one fsnotify watch per directory, and starts the pump.
func New(logger zerolog.Logger, root string, opts ...Option) (*Watcher, error) {
	o := options{debounce: 100 * time.Millisecond, skipDir: defaultSkipDir}
	for _, opt := range opts {
		opt(&o)
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("fswatch: resolve %s: %w", root, err)
	}
	root = abs
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("fswatch: stat root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("fswatch: %s is not a directory", root)
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("fswatch: start watcher: %w", err)
	}

	w := &Watcher{
		logger:  logutils.Component(logger, "fswatch").With().Str("root", root).Logger(),
		root:    root,
		opts:    o,
		fsw:     fsw,
		changes: make(chan Change),
		dirs:    map[string]struct{}{},
		pending: map[string]struct{}{},
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	if err := w.addTree(root); err != nil {
		_ = fsw.Close()
		return nil, fmt.Errorf("fswatch: watch %s: %w", root, err)
	}

	go w.run()
	return w, nil
}

// Changes is closed after Close.
func (w *Watcher) Changes() <-chan Change { return w.changes }

// Close stops the pump and releases every watch. It is idempotent.
func (w *Watcher) Close() error {
	var err error
	w.closeOnce.Do(func() {
		close(w.stop)
		err = w.fsw.Close()
		<-w.done
	})
	return err
}

func (w *Watcher) run() {
	defer close(w.done)
	defer close(w.changes)

	var (
		window <-chan time.Time
		ready  map[string]struct{}
		next   Change
	)
	for {
		var out chan<- Change
		if ready != nil {
			out = w.changes
		}
		select {
		case <-w.stop:
			return
		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			w.handle(event)
			if len(w.pending) > 0 && window == nil {
				window = time.After(w.opts.debounce)
			}
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			w.logger.Warn().Err(err).Msg("watch error")
		case <-window:
			window = nil
			if len(w.pending) == 0 {
				continue
			}
			if ready == nil {
				ready = map[string]struct{}{}
			}
			maps.Copy(ready, w.pending)
			clear(w.pending)
			next = Change{Dirs: slices.Sorted(maps.Keys(ready))}
		case out <- next:
			ready = nil
			next = Change{}
		}
	}
}

func (w *Watcher) handle(event fsnotify.Event) {
	if event.Op&relevantOps == 0 {
		return
	}
	name := filepath.Clean(event.Name)
	_, tracked := w.dirs[name]
	parent := filepath.Dir(name)
	if name == w.root {
		parent = w.root
	}
	if _, ok := w.dirs[parent]; !ok && !tracked {
		// The watch that produced this event was dropped already.
		return
	}

	if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
		if tracked {
			w.dropTree(name)
		}
		w.pending[parent] = struct{}{}
		return
	}
	if event.Has(fsnotify.Create) {
		w.pending[parent] = struct{}{}
		if !w.watchable(name) {
			return
		}
		switch err := w.addTree(name); {
		case errors.Is(err, fs.ErrNotExist):
			w.logger.Debug().Str("dir", name).Msg("created directory vanished before its watch")
		case err != nil:
			w.logger.Warn().Err(err).Str("dir", name).Msg("watch created directory failed")
		default:
			w.pending[name] = struct{}{}
		}
		return
	}
	if tracked {
		w.pending[name] = struct{}{}
	} else {
		w.pending[parent] = struct{}{}
	}
}

// watchable reports whether a created entry is a directory the watcher should
// add. Lstat keeps a link to a directory out, matching the walk.
func (w *Watcher) watchable(name string) bool {
	if w.opts.skipDir(filepath.Base(name)) {
		return false
	}
	info, err := os.Lstat(name)
	return err == nil && info.IsDir()
}

// addTree watches dir before reading it, so a file created after the watch
// exists produces an event and one created before it is found by the
// consumer's read of dir.
func (w *Watcher) addTree(dir string) error {
	if _, ok := w.dirs[dir]; ok {
		return nil
	}
	if err := w.fsw.Add(dir); err != nil {
		return err
	}
	w.dirs[dir] = struct{}{}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || w.opts.skipDir(entry.Name()) {
			continue
		}
		child := filepath.Join(dir, entry.Name())
		switch err := w.addTree(child); {
		case errors.Is(err, fs.ErrNotExist):
			w.logger.Debug().Str("dir", child).Msg("directory vanished during walk")
		case err != nil:
			w.logger.Warn().Err(err).Str("dir", child).Msg("watch directory failed")
		}
	}
	return nil
}

// dropTree forgets dir and every tracked directory below it. fsnotify has
// already released the watch of a directory it reported removed or renamed,
// but the watches below a renamed directory stay registered under their old
// paths, and on kqueue each one holds a descriptor per file, so they are
// released here. The renamed directory's own per-file descriptors are out of
// reach until those files are deleted or the watcher closes.
func (w *Watcher) dropTree(dir string) {
	prefix := dir + string(filepath.Separator)
	for tracked := range w.dirs {
		if tracked != dir && !strings.HasPrefix(tracked, prefix) {
			continue
		}
		delete(w.dirs, tracked)
		delete(w.pending, tracked)
		err := w.fsw.Remove(tracked)
		if err != nil && !errors.Is(err, fsnotify.ErrNonExistentWatch) && !errors.Is(err, fsnotify.ErrClosed) {
			w.logger.Debug().Err(err).Str("dir", tracked).Msg("unwatch directory failed")
		}
	}
}
