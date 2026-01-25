package jsonfile

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/hay-kot/hive/internal/core/history"
)

// HistoryFile is the root JSON structure stored on disk.
type HistoryFile struct {
	Entries []history.Entry `json:"entries"`
}

// HistoryStore implements history.Store using a JSON file for persistence.
type HistoryStore struct {
	path string
	mu   sync.RWMutex
}

// NewHistoryStore creates a new JSON file history store at the given path.
func NewHistoryStore(path string) *HistoryStore {
	return &HistoryStore{path: path}
}

// List returns all history entries, newest first.
func (s *HistoryStore) List(ctx context.Context) ([]history.Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	file, err := s.load()
	if err != nil {
		return nil, err
	}

	return file.Entries, nil
}

// Get returns a history entry by ID. Returns ErrNotFound if not found.
func (s *HistoryStore) Get(ctx context.Context, id string) (history.Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	file, err := s.load()
	if err != nil {
		return history.Entry{}, err
	}

	for _, entry := range file.Entries {
		if entry.ID == id {
			return entry, nil
		}
	}

	return history.Entry{}, history.ErrNotFound
}

// Save adds a new history entry, pruning old entries to stay within maxEntries.
func (s *HistoryStore) Save(ctx context.Context, entry history.Entry, maxEntries int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		return err
	}

	// Prepend new entry (newest first)
	file.Entries = append([]history.Entry{entry}, file.Entries...)

	// Prune to max entries
	if maxEntries > 0 && len(file.Entries) > maxEntries {
		file.Entries = file.Entries[:maxEntries]
	}

	return s.save(file)
}

// Clear removes all history entries.
func (s *HistoryStore) Clear(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.save(HistoryFile{Entries: []history.Entry{}})
}

// LastFailed returns the most recent failed entry. Returns ErrNotFound if none.
func (s *HistoryStore) LastFailed(ctx context.Context) (history.Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	file, err := s.load()
	if err != nil {
		return history.Entry{}, err
	}

	for _, entry := range file.Entries {
		if entry.Failed() {
			return entry, nil
		}
	}

	return history.Entry{}, history.ErrNotFound
}

// load reads the history file from disk.
// Returns empty HistoryFile if file doesn't exist.
func (s *HistoryStore) load() (HistoryFile, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return HistoryFile{}, nil
		}
		return HistoryFile{}, err
	}

	if len(data) == 0 {
		return HistoryFile{}, nil
	}

	var file HistoryFile
	if err := json.Unmarshal(data, &file); err != nil {
		return HistoryFile{}, err
	}

	return file, nil
}

// save writes the history file to disk atomically.
func (s *HistoryStore) save(file HistoryFile) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}

	return os.Rename(tmp, s.path)
}
