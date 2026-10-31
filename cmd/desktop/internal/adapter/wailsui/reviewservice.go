package wailsui

import (
	"context"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/internal/platform/observe"
	"github.com/colonyops/hive/pkg/logutils"
)

// ReviewService exposes comments on Drive Markdown documents and their
// delivery as feedback, to the clipboard or to a running agent. space and
// path name a document the way DriveService does.
type ReviewService struct {
	review *app.ReviewService
	logger zerolog.Logger
}

func NewReviewService(r *app.ReviewService, logger zerolog.Logger) *ReviewService {
	return &ReviewService{review: r, logger: logutils.Component(logger, "wails.review")}
}

func (s *ReviewService) Review(ctx context.Context, space, path string) (DocumentReviewWire, error) {
	r, err := s.review.Review(ctx, space, path)
	if err != nil {
		return DocumentReviewWire{}, s.fail(ctx, "review", err)
	}
	return documentReviewWireOf(r), nil
}

func (s *ReviewService) AddComment(ctx context.Context, space, path string, in app.CommentInput) (_ CommentWire, err error) {
	ctx, span := startReviewCommand(ctx, "add-comment", space, path)
	defer observe.End(span, &err)
	c, err := s.review.AddComment(ctx, space, path, in)
	if err != nil {
		return CommentWire{}, s.fail(ctx, "add-comment", err)
	}
	return commentWireOf(c), nil
}

func (s *ReviewService) UpdateComment(ctx context.Context, space, path, id, text string) (err error) {
	ctx, span := startReviewCommand(ctx, "update-comment", space, path)
	defer observe.End(span, &err)
	return s.fail(ctx, "update-comment", s.review.UpdateComment(ctx, space, path, id, text))
}

func (s *ReviewService) DeleteComment(ctx context.Context, space, path, id string) (err error) {
	ctx, span := startReviewCommand(ctx, "delete-comment", space, path)
	defer observe.End(span, &err)
	return s.fail(ctx, "delete-comment", s.review.DeleteComment(ctx, space, path, id))
}

// Feedback is the open review as the text an agent reads, with note leading.
func (s *ReviewService) Feedback(ctx context.Context, space, path, note string) (string, error) {
	text, err := s.review.Feedback(ctx, space, path, note)
	return text, s.fail(ctx, "feedback", err)
}

func (s *ReviewService) Clear(ctx context.Context, space, path string) (err error) {
	ctx, span := startReviewCommand(ctx, "clear", space, path)
	defer observe.End(span, &err)
	return s.fail(ctx, "clear", s.review.Clear(ctx, space, path))
}

// AgentTargets lists the agents Send can deliver to.
func (s *ReviewService) AgentTargets(ctx context.Context) ([]app.AgentTarget, error) {
	targets, err := s.review.AgentTargets(ctx)
	if err != nil {
		return nil, s.fail(ctx, "agent-targets", err)
	}
	return targets, nil
}

// Send pastes the feedback into the agent's pane, submits it, and clears the
// review. The review stays when delivery fails.
func (s *ReviewService) Send(ctx context.Context, space, path, note string, to app.AgentTargetRef) (err error) {
	ctx, span := startReviewCommand(ctx, "send", space, path)
	defer observe.End(span, &err)
	return s.fail(ctx, "send", s.review.Send(ctx, space, path, note, to))
}

// ChangesTargets lists the agents of one Code session.
func (s *ReviewService) ChangesTargets(ctx context.Context, sessionID string) ([]app.AgentTarget, error) {
	targets, err := s.review.ChangesTargets(ctx, sessionID)
	if err != nil {
		return nil, s.fail(ctx, "changes-targets", err)
	}
	return targets, nil
}

// ChangesFeedback builds the text SendChanges delivers, for the clipboard.
func (s *ReviewService) ChangesFeedback(ctx context.Context, comments []app.DiffCommentInput) (string, error) {
	text, err := s.review.ChangesFeedback(ctx, comments)
	if err != nil {
		return "", s.fail(ctx, "changes-feedback", err)
	}
	return text, nil
}

// SendChanges pastes feedback on a session's changes into its agent's pane and
// submits it.
func (s *ReviewService) SendChanges(ctx context.Context, sessionID, pane string, comments []app.DiffCommentInput) (err error) {
	ctx, span := startReviewCommand(ctx, "send-changes", "", sessionID)
	defer observe.End(span, &err)
	return s.fail(ctx, "send-changes", s.review.SendChanges(ctx, sessionID, pane, comments))
}

func startReviewCommand(ctx context.Context, command, space, path string) (context.Context, trace.Span) {
	return startCommand(ctx, "review "+command, space, path)
}

func (s *ReviewService) fail(ctx context.Context, command string, err error) error {
	if err == nil {
		return nil
	}
	if kind := app.KindOf(err); kind != app.KindInternal {
		s.logger.Debug().Ctx(ctx).Err(err).Str("command", command).Str("kind", string(kind)).Msg("review call refused")
		return err
	}
	s.logger.Error().Ctx(ctx).Err(err).Str("command", command).Msg("review call failed")
	return err
}
