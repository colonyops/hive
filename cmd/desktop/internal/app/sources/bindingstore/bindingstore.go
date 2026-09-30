// Package bindingstore persists each connected account's non-secret binding —
// the host, stack, or project a credential was connected to — keyed by
// credential ref. Binding the host to the account at connect time, rather than
// letting a node name one, is what stops a node from pairing an account's
// token with an arbitrary server.
package bindingstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/colonyops/hive/cmd/desktop/internal/app/credentials"
)

// Store is a mutex-serialized JSON file of ref → binding.
type Store[T any] struct {
	name string
	path string
	mu   sync.Mutex
}

// New returns a store backed by path; name prefixes its errors
// ("gitea instances").
func New[T any](name, path string) *Store[T] {
	return &Store[T]{name: name, path: path}
}

// Get returns the stored binding. A zero binding means nothing is stored — the
// ordinary "not connected" state, not an error.
func (s *Store[T]) Get(ref credentials.Ref) (T, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.load()
	if err != nil {
		var zero T
		return zero, err
	}
	return entries[ref.String()], nil
}

func (s *Store[T]) Set(ref credentials.Ref, binding T) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.load()
	if err != nil {
		return err
	}
	entries[ref.String()] = binding
	return s.write(entries)
}

func (s *Store[T]) Delete(ref credentials.Ref) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.load()
	if err != nil {
		return err
	}
	delete(entries, ref.String())
	return s.write(entries)
}

func (s *Store[T]) load() (map[string]T, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]T{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: read %s: %w", s.name, s.path, err)
	}
	var entries map[string]T
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("%s: parse %s: %w", s.name, s.path, err)
	}
	if entries == nil {
		entries = map[string]T{}
	}
	return entries, nil
}

// write replaces the file atomically: a temp file in the same directory, then a
// rename, so a crash mid-write cannot truncate the store.
func (s *Store[T]) write(entries map[string]T) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("%s: create dir: %w", s.name, err)
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("%s: encode: %w", s.name, err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("%s: write temp: %w", s.name, err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("%s: replace: %w", s.name, err)
	}
	return nil
}
