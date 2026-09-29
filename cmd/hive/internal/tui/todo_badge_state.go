package tui

// todoBadgeState tracks the header badge state for todo counts.
type todoBadgeState struct {
	pending  int
	open     int
	degraded bool
}

func (s *todoBadgeState) updateCounts(pending, open int) {
	s.pending = pending
	s.open = open
	s.degraded = false
}

func (s *todoBadgeState) markDegraded() {
	s.degraded = true
}

func (s *todoBadgeState) clearPendingWithOpen(open int) {
	s.pending = 0
	s.open = open
	s.degraded = false
}
