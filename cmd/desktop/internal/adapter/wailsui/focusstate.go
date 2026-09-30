package wailsui

import "sync"

// FocusState holds the current native window focus state. Exported because
// the notification gate and the window hooks both hold it.
type FocusState struct {
	mu      sync.RWMutex
	focused bool
}

func NewFocusState() *FocusState {
	return &FocusState{}
}

// Set updates the focus state and reports whether it changed.
func (s *FocusState) Set(focused bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.focused == focused {
		return false
	}
	s.focused = focused
	return true
}

// Get reports the current focus state.
func (s *FocusState) Get() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.focused
}
