// Package diffreview is the service behind agents' reviews of a hive session's
// branch diff. Any agent writes one, through the hive CLI or Hive Desktop; the
// desktop shows it inline in the session's Diff view.
package diffreview

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/domain/review"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/gitstatus"
	"github.com/colonyops/hive/pkg/logutils"
)

// ErrInvalid marks a review the service refuses: the message says what to fix.
var ErrInvalid = errors.New("invalid diff review")

type invalidError struct{ msg string }

func (e invalidError) Error() string      { return e.msg }
func (invalidError) Is(target error) bool { return target == ErrInvalid }

func invalidf(format string, args ...any) error {
	return invalidError{msg: fmt.Sprintf(format, args...)}
}

const (
	maxComments    = 200
	maxAuthorLen   = 80
	maxTextLen     = 8000
	maxSummaryLen  = 8000
	maxAgentLen    = 40
	maxQuotedLines = 200
)

type Sessions interface {
	GetSession(ctx context.Context, id string) (session.Session, error)
}

type Changes interface {
	ReadChanges(ctx context.Context, sess session.Session, target gitstatus.DiffTarget) (gitstatus.Changes, error)
	ReadFileDiffs(ctx context.Context, sess session.Session, target gitstatus.DiffTarget, paths []string) ([]gitstatus.FileDiff, error)
}

type Commits interface {
	ResolveCommit(ctx context.Context, dir, rev string) (string, error)
}

type Service struct {
	logger   zerolog.Logger
	store    review.DiffReviewStore
	sessions Sessions
	changes  Changes
	commits  Commits
	now      func() time.Time
}

func NewService(logger zerolog.Logger, store review.DiffReviewStore, sessions Sessions, changes Changes, commits Commits) *Service {
	return &Service{
		logger: logutils.Component(logger, "diffreview"), store: store,
		sessions: sessions, changes: changes, commits: commits, now: time.Now,
	}
}

// CommentInput is one comment as an agent writes it. A comment on rows names
// the first and last line, each on a side: additions numbers lines as the
// branch has them now, deletions as the base had them. The sides default to
// additions and EndLine to StartLine. Kind file comments on the whole file and
// takes no lines.
type CommentInput struct {
	Kind      review.DiffCommentKind
	Path      string
	StartSide review.DiffSide
	StartLine int
	EndSide   review.DiffSide
	EndLine   int
	Text      string
	Severity  review.Severity
}

type PutInput struct {
	SessionID string
	Author    string
	Agent     string
	Summary   string
	Comments  []CommentInput
}

// Put stores a review of the session's branch diff, replacing the author's
// earlier one. Every comment must name a file in that diff and, unless it is a
// file comment, lines that are rows of it: the desktop can only draw a comment
// on a row it shows. Put quotes those rows itself.
func (s *Service) Put(ctx context.Context, in PutInput) (review.DiffReview, error) {
	author := strings.TrimSpace(in.Author)
	summary := strings.TrimSpace(in.Summary)
	agent := strings.ToLower(strings.TrimSpace(in.Agent))
	switch {
	case author == "":
		return review.DiffReview{}, invalidf("a review needs an author")
	case len(author) > maxAuthorLen:
		return review.DiffReview{}, invalidf("author is longer than %d characters", maxAuthorLen)
	case len(agent) > maxAgentLen:
		return review.DiffReview{}, invalidf("agent is longer than %d characters", maxAgentLen)
	case len(summary) > maxSummaryLen:
		return review.DiffReview{}, invalidf("summary is longer than %d characters", maxSummaryLen)
	case len(in.Comments) > maxComments:
		return review.DiffReview{}, invalidf("a review holds at most %d comments; it has %d", maxComments, len(in.Comments))
	case summary == "" && len(in.Comments) == 0:
		return review.DiffReview{}, invalidf("a review needs a summary or at least one comment")
	}

	sess, err := s.sessions.GetSession(ctx, in.SessionID)
	if err != nil {
		return review.DiffReview{}, fmt.Errorf("get session %s: %w", in.SessionID, err)
	}
	if sess.State != session.StateActive || sess.Path == "" {
		return review.DiffReview{}, invalidf("session %s is %s and has no checkout to review", sess.Name, sess.State)
	}
	target := gitstatus.DiffTarget{Base: gitstatus.DiffBaseBranch}
	changes, err := s.changes.ReadChanges(ctx, sess, target)
	if err != nil {
		return review.DiffReview{}, fmt.Errorf("read the changes of %s: %w", sess.Name, err)
	}
	if changes.Base != gitstatus.DiffBaseBranch {
		return review.DiffReview{}, invalidf("session %s has no branch to compare against %s: it is on the default branch, or git finds no merge base",
			sess.Name, orDefault(changes.DefaultBranch, "the default branch"))
	}
	head, err := s.commits.ResolveCommit(ctx, sess.Path, "HEAD")
	if err != nil {
		return review.DiffReview{}, fmt.Errorf("resolve HEAD of %s: %w", sess.Name, err)
	}

	comments, err := s.comments(ctx, sess, target, changes, in.Comments)
	if err != nil {
		return review.DiffReview{}, err
	}
	r := review.DiffReview{
		ID: uuid.NewString(), SessionID: sess.ID, Author: author, Agent: agent, Head: head,
		Summary: summary, CreatedAt: s.now(), Comments: comments,
	}
	if err := s.store.ReplaceDiffReview(ctx, r); err != nil {
		return review.DiffReview{}, err
	}
	s.logger.Info().Str("session_id", sess.ID).Str("author", author).Int("comments", len(comments)).Msg("diff review stored")
	return r, nil
}

func (s *Service) comments(ctx context.Context, sess session.Session, target gitstatus.DiffTarget, changes gitstatus.Changes, in []CommentInput) ([]review.DiffReviewComment, error) {
	changed := make(map[string]bool, len(changes.Files))
	for _, f := range changes.Files {
		changed[f.Path] = true
	}
	against := orDefault(changes.DefaultBranch, "the default branch")
	var problems []string
	problem := func(i int, c CommentInput, format string, args ...any) {
		problems = append(problems, fmt.Sprintf("comment %d (%s): %s", i+1, c.Path, fmt.Sprintf(format, args...)))
	}

	comments := make([]CommentInput, len(in))
	var linePaths []string
	for i, c := range in {
		c = normalize(c)
		comments[i] = c
		switch {
		case c.Path == "":
			problem(i, c, "a comment needs a path")
		case c.Text == "":
			problem(i, c, "a comment needs text")
		case len(c.Text) > maxTextLen:
			problem(i, c, "text is longer than %d characters", maxTextLen)
		case !c.Severity.Valid():
			problem(i, c, "severity %q is not one of note, suggestion, issue", c.Severity)
		case c.Kind != review.DiffCommentLines && c.Kind != review.DiffCommentFile:
			problem(i, c, "kind %q is not lines or file", c.Kind)
		case !changed[c.Path]:
			problem(i, c, "the file is not in the branch diff against %s", against)
		case c.Kind == review.DiffCommentFile:
		case !validSide(c.StartSide) || !validSide(c.EndSide):
			problem(i, c, "a side is additions or deletions")
		case c.StartLine < 1 || c.EndLine < 1:
			problem(i, c, "lines start at 1")
		case !slices.Contains(linePaths, c.Path):
			linePaths = append(linePaths, c.Path)
		}
	}
	if len(problems) > 0 {
		return nil, invalidf("%s", strings.Join(problems, "\n"))
	}

	rows, err := s.rows(ctx, sess, target, linePaths)
	if err != nil {
		return nil, err
	}
	out := make([]review.DiffReviewComment, 0, len(comments))
	for i, c := range comments {
		comment := review.DiffReviewComment{ID: uuid.NewString(), Kind: c.Kind, Path: c.Path, Text: c.Text, Severity: c.Severity}
		if c.Kind == review.DiffCommentLines {
			fileRows, ok := rows[c.Path]
			if !ok {
				problem(i, c, "git shows no text diff for this file; comment on the whole file with kind file")
				continue
			}
			quote, ok := review.DiffSpanQuote(fileRows, c.StartSide, c.StartLine, c.EndSide, c.EndLine)
			if !ok {
				problem(i, c, "%s is not a run of rows in the branch diff; comment on changed or context lines, or on the whole file with kind file", spanLabel(c))
				continue
			}
			if strings.Count(quote, "\n")+1 > maxQuotedLines {
				problem(i, c, "%s covers more than %d rows; comment on the whole file instead", spanLabel(c), maxQuotedLines)
				continue
			}
			comment.StartSide, comment.StartLine = c.StartSide, c.StartLine
			comment.EndSide, comment.EndLine = c.EndSide, c.EndLine
			comment.Quote = quote
		}
		out = append(out, comment)
	}
	if len(problems) > 0 {
		return nil, invalidf("%s", strings.Join(problems, "\n"))
	}
	return out, nil
}

func normalize(c CommentInput) CommentInput {
	c.Text = strings.TrimSpace(c.Text)
	c.Path = strings.TrimPrefix(strings.TrimSpace(c.Path), "./")
	if c.Severity == "" {
		c.Severity = review.SeveritySuggestion
	}
	if c.Kind == review.DiffCommentFile {
		c.StartSide, c.StartLine, c.EndSide, c.EndLine = "", 0, "", 0
		return c
	}
	if c.StartSide == "" {
		c.StartSide = review.DiffSideAdditions
	}
	if c.EndSide == "" {
		c.EndSide = c.StartSide
	}
	if c.EndLine == 0 {
		c.EndLine = c.StartLine
	}
	return c
}

// rows leaves out a path whose diff is not whole text: binary, truncated, or
// gone since the change list was read.
func (s *Service) rows(ctx context.Context, sess session.Session, target gitstatus.DiffTarget, paths []string) (map[string][]review.DiffRow, error) {
	out := make(map[string][]review.DiffRow, len(paths))
	if len(paths) == 0 {
		return out, nil
	}
	diffs, err := s.changes.ReadFileDiffs(ctx, sess, target, paths)
	if err != nil {
		return nil, fmt.Errorf("read the diffs of %s: %w", sess.Name, err)
	}
	for _, d := range diffs {
		if d.State != gitstatus.FileDiffStateText {
			s.logger.Debug().Str("path", d.Path).Str("state", d.State.String()).Msg("no rows to anchor a diff review comment")
			continue
		}
		out[d.Path] = review.ParseDiffRows(d.Text)
	}
	return out, nil
}

// List returns the session's reviews, oldest first.
func (s *Service) List(ctx context.Context, sessionID string) ([]review.DiffReview, error) {
	if _, err := s.sessions.GetSession(ctx, sessionID); err != nil {
		return nil, fmt.Errorf("get session %s: %w", sessionID, err)
	}
	return s.store.ListDiffReviews(ctx, sessionID)
}

// Dismiss removes one comment. The review goes with its last comment.
func (s *Service) Dismiss(ctx context.Context, commentID string) error {
	return s.store.DeleteDiffReviewComment(ctx, commentID)
}

// DismissReview removes one review and its comments.
func (s *Service) DismissReview(ctx context.Context, reviewID string) error {
	return s.store.DeleteDiffReview(ctx, reviewID)
}

// Clear removes the session's reviews: every one, or only author's when
// author is set. It returns how many it removed.
func (s *Service) Clear(ctx context.Context, sessionID, author string) (int, error) {
	if _, err := s.sessions.GetSession(ctx, sessionID); err != nil {
		return 0, fmt.Errorf("get session %s: %w", sessionID, err)
	}
	if author == "" {
		return s.store.DeleteDiffReviews(ctx, sessionID)
	}
	return s.store.DeleteDiffReviewByAuthor(ctx, sessionID, author)
}

func (s *Service) Fingerprint(ctx context.Context) (review.DiffReviewFingerprint, error) {
	return s.store.DiffReviewFingerprint(ctx)
}

func validSide(side review.DiffSide) bool {
	return side == review.DiffSideAdditions || side == review.DiffSideDeletions
}

func spanLabel(c CommentInput) string {
	old := func(side review.DiffSide) string {
		if side == review.DiffSideDeletions {
			return "old "
		}
		return ""
	}
	if c.StartSide != c.EndSide {
		return fmt.Sprintf("%sL%d to %sL%d", old(c.StartSide), c.StartLine, old(c.EndSide), c.EndLine)
	}
	if c.StartLine == c.EndLine {
		return fmt.Sprintf("%sL%d", old(c.StartSide), c.StartLine)
	}
	return fmt.Sprintf("%sL%d-%d", old(c.StartSide), c.StartLine, c.EndLine)
}

func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
