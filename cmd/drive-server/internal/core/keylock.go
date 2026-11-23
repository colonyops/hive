package core

import "sync"

// keyLock is a mutex per key. An entry lives only while someone holds or
// waits for it, so the map does not grow with every key ever used.
type keyLock struct {
	mu      sync.Mutex
	entries map[string]*keyLockEntry
}

type keyLockEntry struct {
	mu   sync.Mutex
	refs int
}

func (l *keyLock) lock(key string) (unlock func()) {
	l.mu.Lock()
	if l.entries == nil {
		l.entries = map[string]*keyLockEntry{}
	}
	e := l.entries[key]
	if e == nil {
		e = &keyLockEntry{}
		l.entries[key] = e
	}
	e.refs++
	l.mu.Unlock()

	e.mu.Lock()
	return func() {
		e.mu.Unlock()
		l.mu.Lock()
		if e.refs--; e.refs == 0 {
			delete(l.entries, key)
		}
		l.mu.Unlock()
	}
}
