package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/pkg/pathutil"
)

const watcherDebounce = 250 * time.Millisecond

// ErrWatcherClosed indicates that a workspace watcher was closed.
var ErrWatcherClosed = fsnotify.ErrClosed

// Watcher reports changes that can alter the repositories discovered in a
// configured workspace directory.
type Watcher struct {
	fs       *fsnotify.Watcher
	roots    []string
	debounce time.Duration
}

// NewWatcher creates a watcher for configured workspace directories.
func NewWatcher(dirs []string) (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create filesystem watcher: %w", err)
	}

	w := &Watcher{
		fs:       fsWatcher,
		debounce: watcherDebounce,
	}
	for _, dir := range dirs {
		root, err := filepath.Abs(pathutil.ExpandHome(dir))
		if err != nil {
			log.Debug().Err(err).Str("dir", dir).Msg("failed to resolve workspace watch path")
			continue
		}
		w.roots = append(w.roots, filepath.Clean(root))
	}

	w.refreshWatches()
	return w, nil
}

// Wait blocks until repository discovery may need to run again.
func (w *Watcher) Wait() error {
	var timer *time.Timer
	var debounce <-chan time.Time

	for {
		select {
		case event, ok := <-w.fs.Events:
			if !ok {
				return ErrWatcherClosed
			}
			if !w.relevant(event) {
				continue
			}

			w.refreshWatches()
			if timer == nil {
				timer = time.NewTimer(w.debounce)
				debounce = timer.C
				continue
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(w.debounce)

		case err, ok := <-w.fs.Errors:
			if !ok {
				return ErrWatcherClosed
			}
			if timer != nil {
				timer.Stop()
			}
			return fmt.Errorf("watch workspace directories: %w", err)

		case <-debounce:
			return nil
		}
	}
}

func (w *Watcher) relevant(event fsnotify.Event) bool {
	path := filepath.Clean(event.Name)
	for _, root := range w.roots {
		if path == root || isAncestorOfRoot(path, root) {
			return true
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}

		parts := strings.Split(rel, string(filepath.Separator))
		switch {
		case len(parts) == 1:
			return true
		case len(parts) == 2 && parts[1] == ".git":
			return true
		case len(parts) == 3 && parts[1] == ".git" && parts[2] == "config":
			return true
		}
	}
	return false
}

func isAncestorOfRoot(path, root string) bool {
	rel, err := filepath.Rel(path, root)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (w *Watcher) refreshWatches() {
	for _, root := range w.roots {
		parent := nearestExistingDir(filepath.Dir(root))
		if parent != "" {
			if err := w.fs.Add(parent); err != nil {
				log.Debug().Err(err).Str("dir", parent).Msg("failed to watch workspace ancestor directory")
			}
		}

		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			continue
		}
		if err := w.fs.Add(root); err != nil {
			log.Debug().Err(err).Str("dir", root).Msg("failed to watch workspace directory")
			continue
		}

		entries, err := os.ReadDir(root)
		if err != nil {
			log.Debug().Err(err).Str("dir", root).Msg("failed to inspect workspace directory for watches")
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			child := filepath.Join(root, entry.Name())
			if err := w.fs.Add(child); err != nil {
				log.Debug().Err(err).Str("dir", child).Msg("failed to watch workspace child directory")
				continue
			}

			gitDir := filepath.Join(child, ".git")
			if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
				if err := w.fs.Add(gitDir); err != nil {
					log.Debug().Err(err).Str("dir", gitDir).Msg("failed to watch repository metadata")
				}
			}
		}
	}
}

func nearestExistingDir(path string) string {
	for {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			return ""
		}
		path = parent
	}
}

// Close stops the watcher.
func (w *Watcher) Close() error {
	return w.fs.Close()
}
