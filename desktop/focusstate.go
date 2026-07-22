package main

import "sync"

// focusState holds the current native window focus state.
type focusState struct {
	mu      sync.RWMutex
	focused bool
}

func newFocusState() *focusState {
	return &focusState{}
}

// set updates the focus state and reports whether it changed.
func (s *focusState) set(focused bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.focused == focused {
		return false
	}
	s.focused = focused
	return true
}

func (s *focusState) get() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.focused
}
