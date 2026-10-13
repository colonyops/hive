// Package fswatch watches a directory tree and reports which directories
// changed. fsnotify has no recursive watch, so the watcher holds one watch per
// directory and keeps that set in step with the tree. It reports directories
// rather than files because its consumer re-reads a changed directory instead
// of applying event deltas.
//
// Symbolic links below the root are not followed: a linked directory is
// neither walked nor watched. The root itself may be a link. Directories whose
// names start with "." are not watched, which keeps .git, .obsidian, and .hive
// out of the set.
package fswatch

import (
	"context"
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

// DefaultDebounce coalesces the write, rename, and chmod burst of one editor
// save into one Change, and so into one rescan.
const DefaultDebounce = 100 * time.Millisecond

// relevantOps leaves out Chmod: an attribute-only change does not change a
// listing or a file's contents.
const relevantOps = fsnotify.Create | fsnotify.Write | fsnotify.Remove | fsnotify.Rename

type options struct {
	debounce time.Duration
	// budget caps the descriptors the watches hold; 0 is no cap.
	budget int
	// events and errors replace fsnotify's channels in tests, which then
	// decide which events the pump sees.
	events <-chan fsnotify.Event
	errors <-chan error
}

type Option func(*options)

// WithDebounce sets the window that collects events into one Change. The
// window opens at the first event after the previous one closed and closes d
// later, so a continuous writer still yields a Change every d. Default
// DefaultDebounce.
func WithDebounce(d time.Duration) Option {
	return func(o *options) { o.debounce = d }
}

func skipDir(name string) bool { return strings.HasPrefix(name, ".") }

// Change lists the absolute directories whose listing or files changed since
// the previous Change, sorted. A created directory is listed itself as well
// as through its parent: files written into it before its watch existed
// produced no events, so the consumer has to read it.
//
// Resync reports that the kernel dropped events, so Dirs lists every watched
// directory and the consumer must re-read everything it holds, not only
// those directories' listings.
type Change struct {
	Dirs   []string
	Resync bool
}

// Watcher delivers one Change per debounce window on an unbuffered channel.
// It never blocks on its consumer: windows that close while the consumer is
// busy merge into the Change it receives next.
//
// When the root itself is removed or renamed, the watcher stops and Changes
// closes, as after Close: every watch is gone with the root, and a new one
// can only be added once something creates the root again. The consumer
// starts a new Watcher then.
type Watcher struct {
	logger  zerolog.Logger
	root    string
	opts    options
	fsw     *fsnotify.Watcher
	changes chan Change

	// dirs, pending, resync, and the budget fields belong to the pump
	// goroutine after New returns. dirs maps a watched directory to the
	// descriptors its watch is counted as holding.
	dirs         map[string]int
	pending      map[string]struct{}
	resync       bool
	descriptors  int
	budgetWarned bool

	closeOnce sync.Once
	stop      chan struct{}
	done      chan struct{}
}

// New walks root, adds one fsnotify watch per directory, and starts the pump.
func New(logger zerolog.Logger, root string, opts ...Option) (*Watcher, error) {
	budget, budgetErr := descriptorBudget()
	o := options{debounce: DefaultDebounce, budget: budget}
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
	if o.events == nil {
		o.events = fsw.Events
	}
	if o.errors == nil {
		o.errors = fsw.Errors
	}

	w := &Watcher{
		logger:  logutils.Component(logger, "fswatch").With().Str("root", root).Logger(),
		root:    root,
		opts:    o,
		fsw:     fsw,
		changes: make(chan Change),
		dirs:    map[string]int{},
		pending: map[string]struct{}{},
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	if budgetErr != nil {
		w.logger.Debug().Err(budgetErr).Int("budget", o.budget).Msg("descriptor limit not read; using the default budget")
	}
	if err := w.addTree(root); err != nil {
		_ = fsw.Close()
		w.holdDescriptors(-w.descriptors)
		return nil, fmt.Errorf("fswatch: watch %s: %w", root, err)
	}

	go w.run()
	return w, nil
}

// Changes is closed after Close, or once the root is removed or renamed.
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
	defer func() { w.holdDescriptors(-w.descriptors) }()

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
		case event, ok := <-w.opts.events:
			if !ok {
				return
			}
			if w.rootGone(event) {
				w.logger.Info().Msg("watched root removed or renamed; watcher stopped")
				return
			}
			w.handle(event)
		case err, ok := <-w.opts.errors:
			if !ok {
				return
			}
			if errors.Is(err, fsnotify.ErrEventOverflow) {
				w.logger.Warn().Err(err).Msg("events lost; resyncing every watched directory")
				overflows.Add(context.Background(), 1)
				w.recover()
			} else {
				w.logger.Warn().Err(err).Msg("watch error")
			}
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
			next = Change{Dirs: slices.Sorted(maps.Keys(ready)), Resync: next.Resync || w.resync}
			w.resync = false
		case out <- next:
			ready = nil
			next = Change{}
		}
		if len(w.pending) > 0 && window == nil {
			window = time.After(w.opts.debounce)
		}
	}
}

func (w *Watcher) rootGone(event fsnotify.Event) bool {
	return filepath.Clean(event.Name) == w.root && (event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename))
}

// recover follows a queue overflow. The walk adds directories created while
// events were being dropped, the stat drops ones removed meanwhile, and every
// watched directory is marked for the consumer to re-read.
func (w *Watcher) recover() {
	for dir := range w.dirs {
		if _, err := os.Lstat(dir); errors.Is(err, fs.ErrNotExist) {
			w.dropTree(dir)
		}
	}
	if err := w.addTree(w.root); err != nil {
		w.logger.Warn().Err(err).Msg("rewalk after overflow failed")
	}
	for dir := range w.dirs {
		w.pending[dir] = struct{}{}
	}
	w.pending[w.root] = struct{}{}
	w.resync = true
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
	if skipDir(filepath.Base(name)) {
		return false
	}
	info, err := os.Lstat(name)
	return err == nil && info.IsDir()
}

// addTree watches dir before reading it, so a file created after the watch
// exists produces an event and one created before it is found by the
// consumer's read of dir. A directory already watched is walked again, which
// is how recover finds the directories it missed. Past the descriptor budget
// a directory is left unwatched rather than failing the walk.
func (w *Watcher) addTree(dir string) error {
	_, watched := w.dirs[dir]
	if !watched {
		if w.opts.budget > 0 && w.descriptors >= w.opts.budget {
			w.warnOverBudget(dir)
			return nil
		}
		if err := w.fsw.Add(dir); err != nil {
			return err
		}
		w.dirs[dir] = 1
		w.holdDescriptors(1)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if !watched {
		// kqueue holds a descriptor per entry of a watched directory.
		w.dirs[dir] += len(entries)
		w.holdDescriptors(len(entries))
	}
	for _, entry := range entries {
		if !entry.IsDir() || skipDir(entry.Name()) {
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
		w.holdDescriptors(-w.dirs[tracked])
		delete(w.dirs, tracked)
		delete(w.pending, tracked)
		err := w.fsw.Remove(tracked)
		if err != nil && !errors.Is(err, fsnotify.ErrNonExistentWatch) && !errors.Is(err, fsnotify.ErrClosed) {
			w.logger.Debug().Err(err).Str("dir", tracked).Msg("unwatch directory failed")
		}
	}
}

// warnOverBudget logs once: the directories past the budget go unwatched for
// the watcher's lifetime, and a change in one is seen only when its parent's
// listing changes or an overflow forces a resync.
func (w *Watcher) warnOverBudget(dir string) {
	if w.budgetWarned {
		return
	}
	w.budgetWarned = true
	budgetExhausted.Add(context.Background(), 1)
	w.logger.Warn().Int("descriptors", w.descriptors).Int("budget", w.opts.budget).Str("dir", dir).
		Msg("descriptor budget reached; directories past it are not watched")
}

func (w *Watcher) holdDescriptors(n int) {
	w.descriptors += n
	heldDescriptors.Add(context.Background(), int64(n))
}
