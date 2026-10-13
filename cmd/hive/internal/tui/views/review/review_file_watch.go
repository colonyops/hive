package review

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
)

const fileWatchDebounce = 100 * time.Millisecond

// contentOps leaves out Chmod, which does not change what the review shows.
const contentOps = fsnotify.Create | fsnotify.Write | fsnotify.Remove | fsnotify.Rename

// FileWatcher refreshes a review of one file outside every drive space. It
// holds a single watch on the file's directory rather than an fswatch tree:
// that directory can be as large as a home directory, and fswatch would walk
// and watch everything below it.
type FileWatcher struct {
	logger zerolog.Logger
	fsw    *fsnotify.Watcher
	doc    Document
}

// FileChangedMsg carries the watched document after it changed on disk.
type FileChangedMsg struct {
	Document Document
}

// WatchFile starts watching doc's directory for changes to doc.
func WatchFile(logger zerolog.Logger, doc Document) (*FileWatcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("watch %s: %w", doc.Path, err)
	}
	if err := fsw.Add(filepath.Dir(doc.Path)); err != nil {
		closeErr := fsw.Close()
		if closeErr != nil {
			logger.Debug().Err(closeErr).Msg("review: file watcher not closed cleanly")
		}
		return nil, fmt.Errorf("watch %s: %w", doc.Path, err)
	}
	return &FileWatcher{logger: logger, fsw: fsw, doc: doc}, nil
}

// Next waits for the next change to the file. An editor's save arrives as a
// burst of writes or a rename and create, so events settle for a short window
// before the file is read again. Nil once the watcher is closed.
func (w *FileWatcher) Next() tea.Cmd {
	if w == nil {
		return nil
	}
	return func() tea.Msg {
		for w.waitForChange() {
			info, err := os.Stat(w.doc.Path)
			if err != nil {
				w.logger.Debug().Err(err).Str("path", w.doc.Path).Msg("review: watched file not readable after a change")
				continue
			}
			doc := w.doc
			doc.ModTime = info.ModTime()
			doc.Content = ""
			doc.RenderedLines = nil
			return FileChangedMsg{Document: doc}
		}
		return nil
	}
}

func (w *FileWatcher) waitForChange() bool {
	name := filepath.Base(w.doc.Path)
	var settle <-chan time.Time
	for {
		select {
		case event, ok := <-w.fsw.Events:
			if !ok {
				return false
			}
			if settle == nil && filepath.Base(event.Name) == name && event.Op&contentOps != 0 {
				settle = time.After(fileWatchDebounce)
			}
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return false
			}
			w.logger.Warn().Err(err).Str("path", w.doc.Path).Msg("review: file watcher error")
		case <-settle:
			return true
		}
	}
}

// Close releases the watch and ends a pending Next.
func (w *FileWatcher) Close() error {
	if w == nil {
		return nil
	}
	return w.fsw.Close()
}
