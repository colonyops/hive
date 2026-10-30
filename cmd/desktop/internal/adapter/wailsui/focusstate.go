package wailsui

import (
	"sync"
	"time"
)

// focusSettleDelay is how long a focus event waits before the state is read.
// Moving between two Hive windows produces a resign and a become-key event, in
// no guaranteed order: Wails hands each to its own goroutine.
const focusSettleDelay = 150 * time.Millisecond

// FocusState answers whether any Hive window is focused. The notification gate
// reads it, and so does every webview through window:focus and window:blur.
//
// An event only says "look again": the answer comes from asking the windows, so
// a late or reordered event cannot leave the state wrong.
type FocusState struct {
	delay  time.Duration
	query  func() bool
	notify func(focused bool)

	mu      sync.Mutex
	focused bool
	timer   *time.Timer
}

// NewFocusState builds the state over query, which reads the live windows.
// notify runs on each transition.
func NewFocusState(delay time.Duration, query func() bool, notify func(focused bool)) *FocusState {
	return &FocusState{delay: delay, query: query, notify: notify}
}

// Changed records that some window's focus may have moved. Calls within the
// delay collapse into one read.
func (s *FocusState) Changed() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(s.delay, s.settle)
}

func (s *FocusState) settle() {
	// Outside the lock: the query hops to the main thread.
	focused := s.query()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.focused == focused {
		return
	}
	s.focused = focused
	s.notify(focused)
}

// Get reports the last settled focus state.
func (s *FocusState) Get() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.focused
}
