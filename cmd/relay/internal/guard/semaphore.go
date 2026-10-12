package guard

import "sync"

// Semaphore bounds how many requests run at once.
type Semaphore struct {
	slots chan struct{}
}

func NewSemaphore(n int) *Semaphore {
	return &Semaphore{slots: make(chan struct{}, n)}
}

// TryAcquire takes a slot without blocking. The returned release is safe to
// call more than once; only the first call frees the slot.
func (s *Semaphore) TryAcquire() (release func(), ok bool) {
	select {
	case s.slots <- struct{}{}:
	default:
		return nil, false
	}
	var once sync.Once
	return func() { once.Do(func() { <-s.slots }) }, true
}
