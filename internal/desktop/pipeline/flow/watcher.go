package flow

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
)

// flowsWatchDebounce coalesces bursts of edits (an editor's write+rename+
// chmod, a git checkout touching several files) into one reload.
const flowsWatchDebounce = 250 * time.Millisecond

// FlowsWatcher invokes onChange when a *.yaml/*.yml file in the flows
// directory changes on disk, so edits made outside the app — or the app's
// own writes via SaveFlow/SaveLayout — apply live. It mirrors
// feed.ConfigWatcher: it watches the directory rather than individual
// files, since editors and atomic writers replace a file by rename (which
// silently drops a watch registered on the file itself), and so the watch
// works before the directory has any flow files in it yet.
type FlowsWatcher struct {
	dir      string
	onChange func()
	logger   zerolog.Logger
	watcher  *fsnotify.Watcher

	stopOnce sync.Once
	stop     chan struct{}
}

func NewFlowsWatcher(dir string, onChange func(), logger zerolog.Logger) (*FlowsWatcher, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("flow: create flows dir: %w", err)
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("flow: start flows watcher: %w", err)
	}
	if err := watcher.Add(dir); err != nil {
		_ = watcher.Close()
		return nil, fmt.Errorf("flow: watch %s: %w", dir, err)
	}
	return &FlowsWatcher{
		dir:      dir,
		onChange: onChange,
		logger:   logger,
		watcher:  watcher,
		stop:     make(chan struct{}),
	}, nil
}

// Start runs the watch loop in a goroutine until Close.
func (w *FlowsWatcher) Start() {
	go w.run()
}

func (w *FlowsWatcher) Close() {
	w.stopOnce.Do(func() {
		close(w.stop)
		if err := w.watcher.Close(); err != nil {
			w.logger.Debug().Err(err).Msg("flows watcher close failed")
		}
	})
}

func (w *FlowsWatcher) run() {
	var debounce <-chan time.Time
	for {
		select {
		case <-w.stop:
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if !isFlowFile(event.Name) {
				continue
			}
			if !event.Op.Has(fsnotify.Create) && !event.Op.Has(fsnotify.Write) &&
				!event.Op.Has(fsnotify.Rename) && !event.Op.Has(fsnotify.Remove) {
				continue
			}
			debounce = time.After(flowsWatchDebounce)
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.logger.Warn().Err(err).Msg("flows watch error")
		case <-debounce:
			debounce = nil
			w.onChange()
		}
	}
}

// isFlowFile reports whether name (as delivered by fsnotify — a path inside
// the watched directory) is a *.yaml/*.yml file. This matches both flow
// definitions and their sibling .ui.yaml layouts: a layout-only edit
// triggers the same (cheap, idempotent) Reload as a flow edit, rather than
// needing to distinguish the two — the same tradeoff feed.ConfigWatcher
// makes for its own writes re-triggering itself.
func isFlowFile(name string) bool {
	ext := filepath.Ext(name)
	return ext == ".yaml" || ext == ".yml"
}
