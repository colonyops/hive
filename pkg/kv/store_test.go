package kv

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStore_GetSet(t *testing.T) {
	s := New[string, int]()

	// Set and get
	s.Set("foo", 42)
	val, ok := s.Get("foo")
	assert.True(t, ok)
	assert.Equal(t, 42, val)

	// Get non-existent
	_, ok = s.Get("bar")
	assert.False(t, ok)
}

func TestStore_Delete(t *testing.T) {
	s := New[string, string]()
	s.Set("key", "value")

	s.Delete("key")

	_, ok := s.Get("key")
	assert.False(t, ok)
}

func TestStore_SetBatch(t *testing.T) {
	s := New[string, int]()

	s.SetBatch(map[string]int{
		"a": 1,
		"b": 2,
		"c": 3,
	})

	assert.Equal(t, 3, s.Len())

	val, _ := s.Get("b")
	assert.Equal(t, 2, val)
}

func TestStore_Clear(t *testing.T) {
	s := New[string, int]()
	s.Set("a", 1)
	s.Set("b", 2)

	s.Clear()

	assert.Equal(t, 0, s.Len())
}

func TestStore_Keys(t *testing.T) {
	s := New[string, int]()
	s.Set("a", 1)
	s.Set("b", 2)

	keys := s.Keys()
	assert.Len(t, keys, 2)
	assert.Contains(t, keys, "a")
	assert.Contains(t, keys, "b")
}

func TestStore_ConcurrentAccess(t *testing.T) {
	s := New[int, int]()
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s.Set(n, n*2)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s.Get(n)
		}(i)
	}

	wg.Wait()

	assert.Equal(t, 100, s.Len())
}
