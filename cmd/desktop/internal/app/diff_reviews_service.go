package app

import (
	"context"
	"errors"
	"strings"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/domain/review"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/diffreview"
	"github.com/colonyops/hive/internal/ops/reviews"
	"github.com/colonyops/hive/pkg/logutils"
)

// DiffReviewsService serves agents' reviews of a Code session's branch diff:
// agents write them through the drive MCP server, the Diff view reads them,
// and the user dismisses a comment or sends it on to the session's agent. The reviews live in hive.db,
// so the hive CLI writes them too; the hive.db probe reports those writes.
type DiffReviewsService struct {
	engine  func() *hive.Engine
	servers func(server string) (serverDiffReviews, error)
	callers *DriveService
	chats   chatSessionResolver
	logger  zerolog.Logger
}

// serverDiffReviews is the review store of a server, which its agents write
// with the hive CLI there.
type serverDiffReviews interface {
	DiffReviews(ctx context.Context, id string) ([]review.DiffReview, error)
	DismissDiffComment(ctx context.Context, commentID string) error
	DismissDiffReview(ctx context.Context, reviewID string) error
}

type DiffReviewsDeps struct {
	Engine func() *hive.Engine
	// Servers reaches the session API of a server under servers:. nil means
	// no session on a server has reviews to show.
	Servers func(server string) (serverDiffReviews, error)
	Callers *DriveService
	Chats   chatSessionResolver
	Logger  zerolog.Logger
}

func newDiffReviewsService(d DiffReviewsDeps) *DiffReviewsService {
	return &DiffReviewsService{
		engine: d.Engine, servers: d.Servers, callers: d.Callers, chats: d.Chats,
		logger: logutils.Component(d.Logger, "desktop.diff-reviews"),
	}
}

func (s *DiffReviewsService) service() (*hive.Engine, error) {
	engine := s.engine()
	if engine == nil {
		return nil, Errorf(KindUnavailable, "the Hive runtime is not open")
	}
	return engine, nil
}

// Put stores the caller's review. The author defaults to the chat's workspace,
// and in a hive session to the agent's name.
func (s *DiffReviewsService) Put(ctx context.Context, in reviews.DiffReviewPutIn) (review.DiffReview, error) {
	engine, err := s.service()
	if err != nil {
		return review.DiffReview{}, err
	}
	target, author, err := s.target(ctx, engine, in.Caller, in.SessionID)
	if err != nil {
		return review.DiffReview{}, err
	}
	if in.Author = strings.TrimSpace(in.Author); in.Author == "" {
		in.Author = author
	}
	if in.Author == "" {
		in.Author = orDefaultString(strings.TrimSpace(in.Agent), "agent")
	}
	comments := make([]diffreview.CommentInput, 0, len(in.Comments))
	for _, c := range in.Comments {
		comments = append(comments, diffreview.CommentInput{
			Kind: reviews.DomainKind(c.Kind), Path: c.Path,
			StartSide: review.DiffSide(c.StartSide), StartLine: c.StartLine,
			EndSide: review.DiffSide(c.EndSide), EndLine: c.EndLine,
			Text: c.Text, Severity: review.Severity(c.Severity),
		})
	}
	r, err := engine.DiffReviews().Put(ctx, diffreview.PutInput{
		SessionID: target.ID, Author: in.Author, Agent: in.Agent, Summary: in.Summary, Comments: comments,
	})
	if err != nil {
		return review.DiffReview{}, diffReviewError(err, target.Name)
	}
	return r, nil
}

// Get lists the reviews of the caller's Code session, or of the one named.
func (s *DiffReviewsService) Get(ctx context.Context, caller, sessionRef string) ([]review.DiffReview, error) {
	engine, err := s.service()
	if err != nil {
		return nil, err
	}
	target, _, err := s.target(ctx, engine, caller, sessionRef)
	if err != nil {
		return nil, err
	}
	return s.List(ctx, target.ID)
}

// Clear removes the reviews of the caller's Code session, or only author's.
func (s *DiffReviewsService) Clear(ctx context.Context, caller, sessionRef, author string) (int, error) {
	engine, err := s.service()
	if err != nil {
		return 0, err
	}
	target, _, err := s.target(ctx, engine, caller, sessionRef)
	if err != nil {
		return 0, err
	}
	n, err := engine.DiffReviews().Clear(ctx, target.ID, strings.TrimSpace(author))
	if err != nil {
		return 0, diffReviewError(err, target.Name)
	}
	return n, nil
}

// List is the Diff view's read of one session's reviews. A "<server>:<id>"
// session's reviews are in that server's hive.db.
func (s *DiffReviewsService) List(ctx context.Context, sessionID string) ([]review.DiffReview, error) {
	server, id, err := s.onServer(sessionID)
	if err != nil {
		return nil, err
	}
	if server != nil {
		readCtx, cancel := context.WithTimeout(ctx, serverReadTimeout)
		defer cancel()
		out, err := server.DiffReviews(readCtx, id)
		if err != nil {
			return nil, serverDiffReviewError(err, "reading the agent reviews of session %q", sessionID)
		}
		return out, nil
	}
	engine, err := s.service()
	if err != nil {
		return nil, err
	}
	out, err := engine.DiffReviews().List(ctx, sessionID)
	if err != nil {
		return nil, diffReviewError(err, sessionID)
	}
	return out, nil
}

// Dismiss removes one agent comment of the session: the user dismissed it, or
// Send delivered it.
func (s *DiffReviewsService) Dismiss(ctx context.Context, sessionID, commentID string) error {
	server, _, err := s.onServer(sessionID)
	if err != nil {
		return err
	}
	if server != nil {
		writeCtx, cancel := context.WithTimeout(ctx, serverReadTimeout)
		defer cancel()
		return serverDiffReviewError(server.DismissDiffComment(writeCtx, commentID), "dismissing a comment of session %q", sessionID)
	}
	engine, err := s.service()
	if err != nil {
		return err
	}
	if err := engine.DiffReviews().Dismiss(ctx, commentID); err != nil {
		return diffReviewError(err, commentID)
	}
	return nil
}

// DismissReview removes one review of the session with every comment it still has.
func (s *DiffReviewsService) DismissReview(ctx context.Context, sessionID, reviewID string) error {
	server, _, err := s.onServer(sessionID)
	if err != nil {
		return err
	}
	if server != nil {
		writeCtx, cancel := context.WithTimeout(ctx, serverReadTimeout)
		defer cancel()
		return serverDiffReviewError(server.DismissDiffReview(writeCtx, reviewID), "dismissing a review of session %q", sessionID)
	}
	engine, err := s.service()
	if err != nil {
		return err
	}
	if err := engine.DiffReviews().DismissReview(ctx, reviewID); err != nil {
		return diffReviewError(err, reviewID)
	}
	return nil
}

// target resolves which Code session a call is about. A hive session's agent
// reviews its own unless it names another; a chat sits beside no Code
// session, so it names one. The second value is the author a chat's review
// defaults to: its workspace.
func (s *DiffReviewsService) target(ctx context.Context, engine *hive.Engine, caller, ref string) (session.Session, string, error) {
	from, err := s.callers.resolve(ctx, caller)
	if err != nil {
		return session.Session{}, "", err
	}
	var author string
	if from.author.Session != "" {
		rec, err := s.chats.Get(ctx, from.author.Session)
		if err != nil {
			return session.Session{}, "", Wrap(err, KindInternal, "loading chat %q", from.author.Session)
		}
		author = rec.Workspace
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		if from.author.HiveSession == "" {
			return session.Session{}, "", Errorf(KindInvalid, "a Hive chat sits beside no Code session; name one in sessionId, by id or name from session_list")
		}
		ref = from.author.HiveSession
	}
	target, err := engine.Sessions().ResolveSession(ctx, ref)
	if errors.Is(err, session.ErrNotFound) {
		return session.Session{}, "", Errorf(KindNotFound, "no hive session has the id or name %q", ref)
	}
	if err != nil {
		return session.Session{}, "", Wrap(err, KindInternal, "finding hive session %q", ref)
	}
	return target, author, nil
}

// onServer resolves a "<server>:<id>" session id to that server's reviews and
// the session's id there. A local session id resolves to nil.
func (s *DiffReviewsService) onServer(sessionID string) (serverDiffReviews, string, error) {
	ref, err := session.ParseRef(sessionID, session.LocalServer)
	if err != nil {
		return nil, "", Wrap(err, KindInvalid, "session id")
	}
	if ref.Server == session.LocalServer {
		return nil, "", nil
	}
	if s.servers == nil {
		return nil, "", Errorf(KindUnavailable, "agent reviews of a session on a server are not available")
	}
	server, err := s.servers(ref.Server)
	if err != nil {
		return nil, "", err
	}
	return server, ref.ID, nil
}

func serverDiffReviewError(err error, format string, args ...any) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, review.ErrDiffReviewNotFound):
		return Errorf(KindNotFound, "that review or comment is gone from the server; it may have been dismissed")
	case errors.Is(err, session.ErrNotFound):
		return Wrap(err, KindNotFound, format, args...)
	default:
		return serverError(err, KindUnavailable, format, args...)
	}
}

func diffReviewError(err error, subject string) error {
	switch {
	case errors.Is(err, diffreview.ErrInvalid):
		return Errorf(KindInvalid, "%s", err.Error())
	case errors.Is(err, review.ErrDiffReviewNotFound):
		return Errorf(KindNotFound, "%s has no such review or comment; it may have been dismissed", subject)
	case errors.Is(err, session.ErrNotFound):
		return Errorf(KindNotFound, "session %q not found", subject)
	default:
		return Wrap(err, KindInternal, "diff review of %s", subject)
	}
}

func orDefaultString(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
