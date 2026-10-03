package credentials

import (
	"fmt"
	"sort"
	"sync"
)

// MemoryStore keeps credentials in memory. It backs mock modes and tests: a
// keychain read can prompt, and a suite that prompts is a suite that hangs in
// CI.
type MemoryStore struct {
	mu     sync.Mutex
	values map[Ref]string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{values: map[Ref]string{}}
}

func (s *MemoryStore) Get(ref Ref) (string, error) {
	if err := ref.Validate(); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[ref]
	if !ok {
		return "", ErrNotFound
	}
	return value, nil
}

func (s *MemoryStore) Set(ref Ref, value string) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	if value == "" {
		return fmt.Errorf("credentials: refusing to store an empty value for %s", ref)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[ref] = value
	return nil
}

func (s *MemoryStore) Delete(ref Ref) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, ref)
	return nil
}

func (s *MemoryStore) List() ([]Ref, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	refs := make([]Ref, 0, len(s.values))
	for ref := range s.values {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].String() < refs[j].String() })
	return refs, nil
}
