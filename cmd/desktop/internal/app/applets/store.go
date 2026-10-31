package applets

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

// Entry is one directory under the root. A broken file keeps the Applet that last
// loaded from it, with the problem in Err, so an edit in progress neither removes
// the applet from the palette nor hides why the edit did not take.
type Entry struct {
	Applet Applet
	Err    error
	// Revision is the newest change among the applet's watched files, in Unix
	// milliseconds. It is what tells a page on screen to load again after an edit,
	// without a change to another applet restarting this one.
	Revision int64
}

// Usable reports that there is a loaded declaration to run.
func (e Entry) Usable() bool { return e.Applet.Version != 0 }

// Store is the catalog. It reads the disk only in Reload, so opening the palette
// costs no I/O.
type Store struct {
	root string

	mu      sync.RWMutex
	entries map[string]Entry
}

func NewStore(root string) *Store {
	return &Store{root: root, entries: map[string]Entry{}}
}

func (s *Store) Root() string { return s.root }

// IsWatchedFile matches what can change the catalog or a page on screen. A shell
// script is read when it runs and needs no reload, and a data file an applet writes
// for itself must not restart it.
func IsWatchedFile(path string) bool {
	if filepath.Base(path) == FileName {
		return true
	}
	switch filepath.Ext(path) {
	case ".html", ".htm", ".js", ".mjs", ".css":
		return true
	}
	return false
}

// Reload rereads every applet. A directory with no applet.yaml is not an applet
// yet, which is the normal state of one an agent is halfway through writing.
func (s *Store) Reload() error {
	dirs, err := os.ReadDir(s.root)
	if errors.Is(err, fs.ErrNotExist) {
		dirs = nil
	} else if err != nil {
		return fmt.Errorf("applets: reading %s: %w", s.root, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	next := make(map[string]Entry, len(dirs))
	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}
		id := dir.Name()
		path := filepath.Join(s.root, id)
		raw, err := os.ReadFile(filepath.Join(path, FileName))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err == nil {
			var applet Applet
			if applet, err = Parse(id, raw); err == nil {
				applet.Dir = path
				next[id] = Entry{Applet: applet, Revision: newestChange(path)}
				continue
			}
		}
		next[id] = Entry{Applet: s.entries[id].Applet, Err: err, Revision: s.entries[id].Revision}
		if !next[id].Usable() {
			next[id] = Entry{Applet: Applet{ID: id, Dir: path, Title: id}, Err: err}
		}
	}
	s.entries = next
	return nil
}

func newestChange(dir string) int64 {
	files, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	var newest int64
	for _, file := range files {
		if !IsWatchedFile(file.Name()) {
			continue
		}
		if info, err := file.Info(); err == nil {
			newest = max(newest, info.ModTime().UnixMilli())
		}
	}
	return newest
}

// List returns every entry ordered by title.
func (s *Store) List() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Entry, 0, len(s.entries))
	for _, entry := range s.entries {
		out = append(out, entry)
	}
	slices.SortFunc(out, func(a, b Entry) int {
		return cmp.Or(cmp.Compare(a.Applet.Title, b.Applet.Title), cmp.Compare(a.Applet.ID, b.Applet.ID))
	})
	return out
}

// Get returns the applet with that id, if it has a declaration to run.
func (s *Store) Get(id string) (Applet, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.entries[id]
	if !ok || !entry.Usable() {
		return Applet{}, false
	}
	return entry.Applet, true
}
