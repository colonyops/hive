package agentws

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
)

// workspaceWatchDebounce coalesces bursts of edits (an editor's write+rename+
// chmod, a git checkout touching several files) into one reload. A third
// copy of the same constant actions/watcher.go and flow/watcher.go each
// declare — deliberate, not an oversight: a shared debounce helper would
// couple three packages to save nine lines.
const workspaceWatchDebounce = 250 * time.Millisecond

// Watcher invokes onChange when the workspace root changes on disk: mcps.yaml
// at the root, or agent-workspace.yaml inside any workspace directory. Unlike
// ActionsWatcher/FlowsWatcher, which each watch one flat directory, this tree
// is nested and fsnotify is not recursive, so Watcher maintains two levels of
// watch: one on root itself (which sees mcps.yaml and workspace directories
// appearing or disappearing) and one per workspace directory (which sees its
// agent-workspace.yaml). Nothing watches deeper — an agent writing into
// docs/, or the generator rewriting CLAUDE.md/.mcp.json/.codex/ on open, is
// invisible to it, which is the desired behaviour, not an omission.
type Watcher struct {
	root     string
	onChange func()
	logger   zerolog.Logger
	watcher  *fsnotify.Watcher

	mu      sync.Mutex
	watched map[string]bool

	stopOnce sync.Once
	stop     chan struct{}
}

// NewWatcher creates root if it does not yet exist (mirroring
// NewActionsWatcher, precedent TestActionsWatcherCreatesMissingDir) so the
// watch is in place before anything is written under it, then resyncs the
// per-workspace watch set against whatever is already there.
func NewWatcher(root string, onChange func(), logger zerolog.Logger) (*Watcher, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("agentws: create workspace root: %w", err)
	}
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("agentws: start workspace watcher: %w", err)
	}
	if err := fsw.Add(root); err != nil {
		_ = fsw.Close()
		return nil, fmt.Errorf("agentws: watch %s: %w", root, err)
	}

	w := &Watcher{
		root:     root,
		onChange: onChange,
		logger:   logger,
		watcher:  fsw,
		watched:  map[string]bool{},
		stop:     make(chan struct{}),
	}
	w.resync()
	return w, nil
}

// Start runs the watch loop in a goroutine until Close.
func (w *Watcher) Start() {
	go w.run()
}

func (w *Watcher) Close() {
	w.stopOnce.Do(func() {
		close(w.stop)
		if err := w.watcher.Close(); err != nil {
			w.logger.Debug().Err(err).Msg("agent workspace watcher close failed")
		}
	})
}

func (w *Watcher) run() {
	var debounce <-chan time.Time
	for {
		select {
		case <-w.stop:
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if w.handle(event) {
				debounce = time.After(workspaceWatchDebounce)
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.logger.Warn().Err(err).Msg("agent workspace watch error")
		case <-debounce:
			debounce = nil
			w.onChange()
		}
	}
}

// handle reports whether event should arm the debounce. A root-level
// create/remove/rename always resyncs the per-workspace watch set, because
// that is the only signal a workspace directory was added or taken away —
// and a changed watch set is itself reload-worthy: a directory that appears
// with its manifest already inside it (an atomic "cp -r", say) produces no
// further event for that manifest.
func (w *Watcher) handle(event fsnotify.Event) bool {
	if !event.Op.Has(fsnotify.Create) && !event.Op.Has(fsnotify.Write) &&
		!event.Op.Has(fsnotify.Rename) && !event.Op.Has(fsnotify.Remove) {
		return false
	}

	changed := w.interesting(filepath.Base(event.Name))
	isRootEntry := filepath.Dir(event.Name) == w.root
	structural := event.Op.Has(fsnotify.Create) || event.Op.Has(fsnotify.Remove) || event.Op.Has(fsnotify.Rename)
	if isRootEntry && structural && w.resync() {
		changed = true
	}
	return changed
}

// interesting reports whether a basename is worth a reload: mcps.yaml at the
// root, or agent-workspace.yaml inside a watched workspace. AGENTS.md is not
// — nothing consumes it between opens — and neither are the generator's own
// outputs (CLAUDE.md, .mcp.json, .codex/, .claude/, .agents/, docs/), which
// live at the same directory level as agent-workspace.yaml.
func (w *Watcher) interesting(name string) bool {
	return name == libraryFileName || name == manifestFileName
}

// resync reconciles the per-workspace watches against the directories on
// disk. It runs at construction and after every root-level create/remove/
// rename. It reports whether the watched set changed, and tolerates the root
// itself having disappeared (dropping every watch rather than spinning on a
// directory that is gone).
func (w *Watcher) resync() bool {
	entries, err := os.ReadDir(w.root)
	if err != nil {
		w.mu.Lock()
		changed := len(w.watched) > 0
		for dir := range w.watched {
			_ = w.watcher.Remove(dir)
		}
		w.watched = map[string]bool{}
		w.mu.Unlock()
		if !os.IsNotExist(err) {
			w.logger.Warn().Err(err).Msg("agent workspace root read failed")
		}
		return changed
	}

	current := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			current[filepath.Join(w.root, entry.Name())] = true
		}
	}

	w.mu.Lock()
	defer w.mu.Unlock()

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
			w.logger.Warn().Err(err).Str("dir", dir).Msg("agent workspace directory watch failed")
			continue
		}
		w.watched[dir] = true
		changed = true
	}
	return changed
}
