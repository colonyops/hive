package store

import (
	"context"
	"fmt"
	"time"

	"github.com/colonyops/hive/internal/domain/review"
	"github.com/colonyops/hive/internal/store/db"
)

// DiffReviewStore implements review.DiffReviewStore using SQLite.
type DiffReviewStore struct {
	db *db.DB
}

var _ review.DiffReviewStore = (*DiffReviewStore)(nil)

func NewDiffReviewStore(db *db.DB) *DiffReviewStore {
	return &DiffReviewStore{db: db}
}

func (s *DiffReviewStore) ReplaceDiffReview(ctx context.Context, r review.DiffReview) error {
	return s.db.WithinTx(ctx, func(ctx context.Context, tx *db.DB) error {
		if _, err := tx.DeleteDiffReviewByAuthor(ctx, db.DeleteDiffReviewByAuthorParams{SessionID: r.SessionID, Author: r.Author}); err != nil {
			return fmt.Errorf("drop earlier diff review: %w", err)
		}
		err := tx.InsertDiffReview(ctx, db.InsertDiffReviewParams{
			ID: r.ID, SessionID: r.SessionID, Author: r.Author, Agent: r.Agent,
			Head: r.Head, Summary: r.Summary, CreatedAt: r.CreatedAt.UnixNano(),
		})
		if err != nil {
			return fmt.Errorf("insert diff review: %w", err)
		}
		for i, c := range r.Comments {
			err := tx.InsertDiffReviewComment(ctx, db.InsertDiffReviewCommentParams{
				ID: c.ID, ReviewID: r.ID, Position: int64(i), Kind: string(c.Kind), Path: c.Path,
				StartSide: string(c.StartSide), StartLine: int64(c.StartLine),
				EndSide: string(c.EndSide), EndLine: int64(c.EndLine),
				Quote: c.Quote, Text: c.Text, Severity: string(c.Severity),
			})
			if err != nil {
				return fmt.Errorf("insert diff review comment: %w", err)
			}
		}
		return nil
	})
}

func (s *DiffReviewStore) ListDiffReviews(ctx context.Context, sessionID string) ([]review.DiffReview, error) {
	q := s.db.Ctx(ctx)
	rows, err := q.ListDiffReviews(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list diff reviews: %w", err)
	}
	commentRows, err := q.ListDiffReviewComments(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list diff review comments: %w", err)
	}
	byReview := make(map[string][]review.DiffReviewComment, len(rows))
	for _, c := range commentRows {
		byReview[c.ReviewID] = append(byReview[c.ReviewID], review.DiffReviewComment{
			ID: c.ID, ReviewID: c.ReviewID, Kind: review.DiffCommentKind(c.Kind), Path: c.Path,
			StartSide: review.DiffSide(c.StartSide), StartLine: int(c.StartLine),
			EndSide: review.DiffSide(c.EndSide), EndLine: int(c.EndLine),
			Quote: c.Quote, Text: c.Text, Severity: review.Severity(c.Severity),
		})
	}
	out := make([]review.DiffReview, 0, len(rows))
	for _, r := range rows {
		out = append(out, review.DiffReview{
			ID: r.ID, SessionID: r.SessionID, Author: r.Author, Agent: r.Agent, Head: r.Head,
			Summary: r.Summary, CreatedAt: time.Unix(0, r.CreatedAt), Comments: byReview[r.ID],
		})
	}
	return out, nil
}

func (s *DiffReviewStore) DeleteDiffReviewComment(ctx context.Context, commentID string) (string, error) {
	var sessionID string
	err := s.db.WithinTx(ctx, func(ctx context.Context, tx *db.DB) error {
		comment, err := tx.GetDiffReviewComment(ctx, commentID)
		if IsNotFoundError(err) {
			return review.ErrDiffReviewNotFound
		}
		if err != nil {
			return fmt.Errorf("get diff review comment: %w", err)
		}
		r, err := tx.GetDiffReview(ctx, comment.ReviewID)
		if err != nil {
			return fmt.Errorf("get diff review: %w", err)
		}
		sessionID = r.SessionID
		if _, err := tx.DeleteDiffReviewComment(ctx, commentID); err != nil {
			return fmt.Errorf("delete diff review comment: %w", err)
		}
		left, err := tx.CountDiffReviewComments(ctx, r.ID)
		if err != nil {
			return fmt.Errorf("count diff review comments: %w", err)
		}
		if left == 0 {
			if _, err := tx.DeleteDiffReview(ctx, r.ID); err != nil {
				return fmt.Errorf("delete emptied diff review: %w", err)
			}
		}
		return nil
	})
	return sessionID, err
}

func (s *DiffReviewStore) DeleteDiffReview(ctx context.Context, reviewID string) error {
	n, err := s.db.Ctx(ctx).DeleteDiffReview(ctx, reviewID)
	if err != nil {
		return fmt.Errorf("delete diff review: %w", err)
	}
	if n == 0 {
		return review.ErrDiffReviewNotFound
	}
	return nil
}

func (s *DiffReviewStore) DeleteDiffReviews(ctx context.Context, sessionID string) (int, error) {
	n, err := s.db.Ctx(ctx).DeleteDiffReviewsBySession(ctx, sessionID)
	if err != nil {
		return 0, fmt.Errorf("delete diff reviews: %w", err)
	}
	return int(n), nil
}

func (s *DiffReviewStore) DeleteDiffReviewByAuthor(ctx context.Context, sessionID, author string) (int, error) {
	n, err := s.db.Ctx(ctx).DeleteDiffReviewByAuthor(ctx, db.DeleteDiffReviewByAuthorParams{SessionID: sessionID, Author: author})
	if err != nil {
		return 0, fmt.Errorf("delete diff review by author: %w", err)
	}
	return int(n), nil
}

func (s *DiffReviewStore) DiffReviewFingerprint(ctx context.Context) (review.DiffReviewFingerprint, error) {
	row, err := s.db.Ctx(ctx).DiffReviewFingerprint(ctx)
	if err != nil {
		return review.DiffReviewFingerprint{}, fmt.Errorf("diff review fingerprint: %w", err)
	}
	return review.DiffReviewFingerprint{Reviews: row.Reviews, Comments: row.Comments, Latest: row.Latest}, nil
}
