// Package kv provides a generic thread-safe key-value store.
package kv

import "sync"

// Store is a thread-safe generic key-value store.
type Store[K comparable, V any] struct {
	mu   sync.RWMutex
	data map[K]V
}

// New creates a new key-value store.
func New[K comparable, V any]() *Store[K, V] {
	return &Store[K, V]{
		data: make(map[K]V),
	}
}

// Get retrieves a value by key.
func (s *Store[K, V]) Get(key K) (V, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.data[key]
	return val, ok
}

// Set stores a value by key.
func (s *Store[K, V]) Set(key K, value V) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
}

// Delete removes a key from the store.
func (s *Store[K, V]) Delete(key K) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}

// SetBatch stores multiple key-value pairs at once.
func (s *Store[K, V]) SetBatch(items map[K]V) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range items {
		s.data[k] = v
	}
}

// Clear removes all entries from the store.
func (s *Store[K, V]) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = make(map[K]V)
}

// Len returns the number of items in the store.
func (s *Store[K, V]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.data)
}

// Keys returns all keys in the store.
func (s *Store[K, V]) Keys() []K {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]K, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}
	return keys
}
