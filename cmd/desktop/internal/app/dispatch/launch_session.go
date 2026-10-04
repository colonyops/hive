package dispatch

import (
	"context"
	"slices"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/activity"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/internal/domain/session"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
)

// SessionCreator is satisfied by *sessionsvc.Service.
type SessionCreator interface {
	CreateFromRequest(context.Context, sessionsvc.LaunchRequest) (session.Session, error)
}

// ItemSessionLinker persists the association between an inbox item and a
// session created for it, so the item can find the session again after a
// restart.
type ItemSessionLinker interface {
	Link(ctx context.Context, sessionID string, ref models.ItemRef) error
}

// RepositoryLauncher creates a hive session for a launch request, then links
// it to the inbox items it came from and records it in the activity log. It
// calls sessions once per launch, so a hive config reload reaches the next one.
type RepositoryLauncher struct {
	sessions func() SessionCreator
	links    ItemSessionLinker
	recorder activity.Recorder
	logger   zerolog.Logger
}

// NewRepositoryLauncher returns a launcher over sessions. links and recorder
// may be nil: the session is still created, with no link and no activity row.
func NewRepositoryLauncher(sessions func() SessionCreator, links ItemSessionLinker, recorder activity.Recorder, logger zerolog.Logger) *RepositoryLauncher {
	return &RepositoryLauncher{sessions: sessions, links: links, recorder: recorder, logger: logger}
}

func (l *RepositoryLauncher) LaunchSession(ctx context.Context, req LaunchSessionRequest) (SessionExecutionOutcome, error) {
	// Tags are presentational, for a reader inside hive, and are never read
	// back. ItemSessionLinker writes the associations this app queries.
	origins := uniqueKnownOrigins(req.Origins)
	tags := slices.Clone(req.Tags)
	for _, origin := range origins {
		tags = append(tags, origin.ExternalID)
	}
	s, err := l.sessions().CreateFromRequest(ctx, sessionsvc.LaunchRequest{
		Name:            req.Name,
		Prompt:          req.Prompt,
		Agent:           req.Agent,
		Repo:            req.Repo,
		CollisionSuffix: req.CollisionSuffix,
		Tags:            tags,
	})
	if err != nil {
		return SessionExecutionOutcome{}, err
	}
	// The session exists either way, so a failed link is logged rather than
	// returned: reporting the launch as failed would be a lie, and would
	// invite a retry that creates a second session.
	if l.links != nil {
		for _, origin := range origins {
			if linkErr := l.links.Link(ctx, s.ID, origin); linkErr != nil {
				l.logger.Warn().Ctx(ctx).Err(linkErr).Str("session_id", s.ID).Str("external_id", origin.ExternalID).Msg("linking session to an inbox item")
			}
		}
	}
	if l.recorder != nil {
		name := req.Name
		if s.Name != "" {
			name = s.Name
		}
		l.recorder.Record(ctx, activity.SessionCreated(name, req.Agent, req.Repo))
	}
	return SessionExecutionOutcome{ID: s.ID, Name: s.Name, Slug: s.Slug, Path: s.Path}, nil
}

func uniqueKnownOrigins(origins []models.ItemRef) []models.ItemRef {
	seen := make(map[models.ItemRef]struct{}, len(origins))
	var unique []models.ItemRef
	for _, origin := range origins {
		if !origin.Known() {
			continue
		}
		if _, exists := seen[origin]; exists {
			continue
		}
		seen[origin] = struct{}{}
		unique = append(unique, origin)
	}
	return unique
}
