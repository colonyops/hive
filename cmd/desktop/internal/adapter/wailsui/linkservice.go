package wailsui

import (
	"strings"
	"sync"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/domain/deeplink"
	"github.com/colonyops/hive/pkg/logutils"
)

// linkSource names how a link reached the app, for the log.
type linkSource string

const (
	linkFromOS             linkSource = "os"
	linkFromSecondInstance linkSource = "second-instance"
)

// LinkService holds the hive:// link the operating system delivered until the
// frontend takes it. A link that launches the app arrives before the webview
// listens, so link:open is only a wake-up and the link itself waits here.
// Only the latest link is kept: a user who clicks twice wants the second place.
//
// Every step is logged at info: links are rare and user-driven, and a link
// that "does nothing" is otherwise impossible to place between the OS, this
// service, and the frontend.
type LinkService struct {
	logger  zerolog.Logger
	mu      sync.Mutex
	pending *deeplink.Location
}

// NewLinkService constructs an empty link service.
func NewLinkService(logger zerolog.Logger) *LinkService {
	return &LinkService{logger: logutils.Component(logger, "links")}
}

// Take returns the link waiting to be opened and clears it, or nil when there
// is none.
func (s *LinkService) Take() *deeplink.Location {
	s.mu.Lock()
	defer s.mu.Unlock()
	loc := s.pending
	s.pending = nil
	if loc != nil {
		s.logger.Info().Interface("link", *loc).Msg("frontend took hive:// link")
	}
	return loc
}

// deliver parses a link the operating system handed the app and queues it for
// the frontend.
func (s *LinkService) deliver(raw string, source linkSource) error {
	loc, err := deeplink.Parse(raw)
	if err != nil {
		s.logger.Warn().Err(err).Str("url", raw).Str("source", string(source)).Msg("ignoring hive:// link")
		return err
	}
	s.mu.Lock()
	replaced := s.pending != nil
	s.pending = &loc
	s.mu.Unlock()
	s.logger.Info().
		Interface("link", loc).
		Str("url", raw).
		Str("source", string(source)).
		Bool("replaced_pending", replaced).
		Msg("received hive:// link")
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
