package wailsui

import (
	"strings"
	"sync"

	"github.com/colonyops/hive/internal/domain/deeplink"
)

// LinkService holds the hive:// link the operating system delivered until the
// frontend takes it. A link that launches the app arrives before the webview
// listens, so link:open is only a wake-up and the link itself waits here.
// Only the latest link is kept: a user who clicks twice wants the second place.
type LinkService struct {
	mu      sync.Mutex
	pending *deeplink.Location
}

// NewLinkService constructs an empty link service.
func NewLinkService() *LinkService {
	return &LinkService{}
}

// Take returns the link waiting to be opened and clears it, or nil when there
// is none.
func (s *LinkService) Take() *deeplink.Location {
	s.mu.Lock()
	defer s.mu.Unlock()
	loc := s.pending
	s.pending = nil
	return loc
}

// deliver parses a link the operating system handed the app and queues it for
// the frontend.
func (s *LinkService) deliver(raw string) error {
	loc, err := deeplink.Parse(raw)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.pending = &loc
	s.mu.Unlock()
	emitLinkOpen()
	return nil
}

// linkArg finds the hive:// link among a launch's arguments.
func linkArg(args []string) (string, bool) {
	for _, arg := range args {
		if strings.HasPrefix(strings.ToLower(arg), deeplink.Scheme+"://") {
			return arg, true
		}
	}
	return "", false
}
