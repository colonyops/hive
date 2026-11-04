package review

import (
	"context"
	"errors"
	"time"
)

// Severity says how much an agent's comment on a diff matters.
type Severity string

const (
	SeverityNote       Severity = "note"
	SeveritySuggestion Severity = "suggestion"
	SeverityIssue      Severity = "issue"
)

func (s Severity) Valid() bool {
	return s == SeverityNote || s == SeveritySuggestion || s == SeverityIssue
}

// ErrDiffReviewNotFound is returned for a review or comment id that names none.
var ErrDiffReviewNotFound = errors.New("diff review not found")

// DiffReview is one agent's review of a hive session's branch diff. A session
// holds at most one per Author. Agent names the coding agent that wrote it
// (claude, codex), which the desktop draws as that agent's mark; it is empty
// when unknown. Head is the session's HEAD commit when the review was written.
type DiffReview struct {
	ID        string
	SessionID string
	Author    string
	Agent     string
	Head      string
	Summary   string
	CreatedAt time.Time
	Comments  []DiffReviewComment
}

// DiffReviewComment is a comment on rows of one file's diff, or on the file as
// a whole when Kind is DiffCommentFile, whose lines are then 0. Quote holds the
// rows as they read when the review was written, so a reader can tell when the
// lines moved.
type DiffReviewComment struct {
	ID        string
	ReviewID  string
	Kind      DiffCommentKind
	Path      string
	StartSide DiffSide
	StartLine int
	EndSide   DiffSide
	EndLine   int
	Quote     string
	Text      string
	Severity  Severity
}

// DiffReviewFingerprint changes whenever a review or comment is written or
// removed, by any process.
type DiffReviewFingerprint struct {
	Reviews  int64
	Comments int64
	Latest   int64
}

type DiffReviewStore interface {
	// ReplaceDiffReview stores r, dropping the session's earlier review by the
	// same author and its comments, in one transaction.
	ReplaceDiffReview(ctx context.Context, r DiffReview) error

	// ListDiffReviews returns the session's reviews oldest first, each with its
	// comments in the order they were written.
	ListDiffReviews(ctx context.Context, sessionID string) ([]DiffReview, error)

	// DeleteDiffReviewComment removes one comment, and its review when that was
	// the review's last comment. It returns the session the comment was on.
	DeleteDiffReviewComment(ctx context.Context, commentID string) (sessionID string, err error)

	// DeleteDiffReview removes one review and its comments.
	DeleteDiffReview(ctx context.Context, reviewID string) error

	// DeleteDiffReviewByAuthor removes the session's review by author, if any,
	// and returns how many it removed.
	DeleteDiffReviewByAuthor(ctx context.Context, sessionID, author string) (int, error)

	// DeleteDiffReviews removes every review of a session and returns how many.
	DeleteDiffReviews(ctx context.Context, sessionID string) (int, error)

	DiffReviewFingerprint(ctx context.Context) (DiffReviewFingerprint, error)
}
