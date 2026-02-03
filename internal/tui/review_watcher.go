package tui

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	tea "charm.land/bubbletea/v2"
)

// documentChangeMsg is sent when documents change on disk.
type documentChangeMsg struct {
	documents []ReviewDocument
}

// DocumentWatcher watches a directory for document changes.
type DocumentWatcher struct {
	watcher     *fsnotify.Watcher
	contextDir  string
	debounceDur time.Duration
}

// NewDocumentWatcher creates a new document watcher.
func NewDocumentWatcher(contextDir string) (*DocumentWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &DocumentWatcher{
		watcher:     watcher,
		contextDir:  contextDir,
		debounceDur: 100 * time.Millisecond,
	}

	// Add the context directory and all subdirectories to the watcher
	if err := w.addRecursive(contextDir); err != nil {
		watcher.Close()
		return nil, err
	}

	return w, nil
}

// Start returns a command that watches for file changes.
func (w *DocumentWatcher) Start() tea.Cmd {
	return func() tea.Msg {
		// Wait for first event
		for {
			select {
			case event, ok := <-w.watcher.Events:
				if !ok {
					return nil
				}

				// Filter out temp files and non-document files
				if w.shouldIgnore(event.Name) {
					continue
				}

				// If it's a directory creation, add it to the watcher
				if event.Has(fsnotify.Create) {
					if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
						w.addRecursive(event.Name)
					}
				}

				// Debounce: wait for changes to settle
				time.Sleep(w.debounceDur)

				// Drain any additional events that arrived during debounce
				drained := false
				for !drained {
					select {
					case <-w.watcher.Events:
						// Discard event
					default:
						drained = true
					}
				}

				// Rescan documents and send update
				docs, err := DiscoverDocuments(w.contextDir)
				if err != nil {
					continue
				}
				return documentChangeMsg{documents: docs}

			case _, ok := <-w.watcher.Errors:
				if !ok {
					return nil
				}
				// Ignore errors, continue watching
			}
		}
	}
}

// addRecursive adds a directory and all its subdirectories to the watcher.
func (w *DocumentWatcher) addRecursive(path string) error {
	return filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip directories we can't read
		}
		if d.IsDir() {
			// Skip hidden directories except .hive
			if strings.HasPrefix(d.Name(), ".") && d.Name() != ".hive" {
				return filepath.SkipDir
			}
			return w.watcher.Add(p)
		}
		return nil
	})
}

// shouldIgnore returns true if the file should be ignored.
func (w *DocumentWatcher) shouldIgnore(path string) bool {
	base := filepath.Base(path)

	// Ignore hidden files except .hive
	if strings.HasPrefix(base, ".") && base != ".hive" {
		return true
	}

	// Ignore common temp extensions
	for _, ext := range []string{".tmp", ".lock", ".swp", ".swx", "~"} {
		if strings.HasSuffix(base, ext) {
			return true
		}
	}

	// Only watch markdown and text files
	ext := filepath.Ext(path)
	if ext != ".md" && ext != ".txt" && ext != "" {
		return true
	}

	return false
}

// Close stops the watcher.
func (w *DocumentWatcher) Close() error {
	return w.watcher.Close()
}
