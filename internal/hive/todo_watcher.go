package hive

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
)

// TodoFileChangeMsg is sent when files change in watched context directories.
type TodoFileChangeMsg struct {
	Changed []string // file paths that were created or modified
	Deleted []string // file paths that were deleted
}

// TodoWatcher watches context directories for file changes and emits
// TodoFileChangeMsg via tea.Cmd. Follows the same fsnotify + debounce + re-subscribe
// pattern as DocumentWatcher.
type TodoWatcher struct {
	watcher     *fsnotify.Watcher
	dirs        []string
	debounceDur time.Duration
	log         zerolog.Logger
}

// NewTodoWatcher creates a watcher for the given context directories.
// Returns nil if no directories exist or fsnotify fails.
func NewTodoWatcher(dirs []string, log zerolog.Logger) *TodoWatcher {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Warn().Err(err).Msg("todo: failed to create fsnotify watcher")
		return nil
	}

	w := &TodoWatcher{
		watcher:     watcher,
		dirs:        dirs,
		debounceDur: 200 * time.Millisecond,
		log:         log.With().Str("component", "todo-watcher").Logger(),
	}

	added := 0
	for _, dir := range dirs {
		if err := w.addRecursive(dir); err != nil {
			w.log.Debug().Err(err).Str("dir", dir).Msg("skipping context directory")
			continue
		}
		added++
	}

	if added == 0 {
		_ = watcher.Close()
		return nil
	}

	return w
}

// Start returns a tea.Cmd that blocks until file changes occur, then returns
// a TodoFileChangeMsg. The caller must re-invoke Start() after processing
// the message to continue watching.
func (w *TodoWatcher) Start() tea.Cmd {
	return func() tea.Msg {
		for {
			select {
			case event, ok := <-w.watcher.Events:
				if !ok {
					return nil
				}

				if w.shouldIgnore(event.Name) {
					continue
				}

				w.log.Debug().
					Str("path", event.Name).
					Str("op", event.Op.String()).
					Msg("file system event")

				// Track new directories for recursive watching
				if event.Has(fsnotify.Create) {
					if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
						_ = w.addRecursive(event.Name)
					}
				}

				// Debounce: wait for changes to settle using a timer
				debounce := time.NewTimer(w.debounceDur)

				// Collect all events during debounce window
				changed := map[string]bool{}
				deleted := map[string]bool{}

				w.classifyEvent(event, changed, deleted)

				// Drain queued events until debounce timer fires
			debounceLoop:
				for {
					select {
					case e := <-w.watcher.Events:
						if !w.shouldIgnore(e.Name) {
							w.classifyEvent(e, changed, deleted)
						}
						if !debounce.Stop() {
							<-debounce.C
						}
						debounce.Reset(w.debounceDur)
					case <-debounce.C:
						break debounceLoop
					}
				}

				// Remove deleted paths from changed set
				for p := range deleted {
					delete(changed, p)
				}

				msg := TodoFileChangeMsg{
					Changed: mapKeys(changed),
					Deleted: mapKeys(deleted),
				}

				if len(msg.Changed) == 0 && len(msg.Deleted) == 0 {
					continue
				}

				return msg

			case err, ok := <-w.watcher.Errors:
				if !ok {
					return nil
				}
				w.log.Error().Err(err).Msg("watcher error")
			}
		}
	}
}

// Close stops the watcher.
func (w *TodoWatcher) Close() error {
	return w.watcher.Close()
}

func (w *TodoWatcher) classifyEvent(event fsnotify.Event, changed, deleted map[string]bool) {
	if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
		deleted[event.Name] = true
	} else {
		changed[event.Name] = true
	}
}

func (w *TodoWatcher) addRecursive(path string) error {
	return filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			w.log.Debug().Err(err).Str("path", p).Msg("skipping path during walk")
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && d.Name() != ".hive" {
				return filepath.SkipDir
			}
			return w.watcher.Add(p)
		}
		return nil
	})
}

func (w *TodoWatcher) shouldIgnore(path string) bool {
	base := filepath.Base(path)

	if strings.HasPrefix(base, ".") && base != ".hive" {
		return true
	}

	for _, ext := range []string{".tmp", ".lock", ".swp", ".swx", "~"} {
		if strings.HasSuffix(base, ext) {
			return true
		}
	}

	ext := filepath.Ext(path)
	if ext != ".md" && ext != ".txt" && ext != "" {
		return true
	}

	return false
}

func mapKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
