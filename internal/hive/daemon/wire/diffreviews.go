package wire

import (
	"github.com/colonyops/hive/internal/domain/review"
	daemonv1 "github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1"
)

func DiffReviewsToProto(reviews []review.DiffReview) []*daemonv1.DiffReview {
	out := make([]*daemonv1.DiffReview, 0, len(reviews))
	for _, r := range reviews {
		comments := make([]*daemonv1.DiffReviewComment, 0, len(r.Comments))
		for _, c := range r.Comments {
			comments = append(comments, &daemonv1.DiffReviewComment{
				Id: c.ID, ReviewId: c.ReviewID, Kind: string(c.Kind), Path: c.Path,
				StartSide: string(c.StartSide), StartLine: int32(c.StartLine), //nolint:gosec // a line number fits
				EndSide: string(c.EndSide), EndLine: int32(c.EndLine), //nolint:gosec // a line number fits
				Quote: c.Quote, Text: c.Text, Severity: string(c.Severity),
			})
		}
		out = append(out, &daemonv1.DiffReview{
			Id: r.ID, SessionId: r.SessionID, Author: r.Author, Agent: r.Agent, Head: r.Head,
			Summary: r.Summary, CreatedAt: timestampToProto(r.CreatedAt), Comments: comments,
		})
	}
	return out
}

func DiffReviewsFromProto(reviews []*daemonv1.DiffReview) []review.DiffReview {
	out := make([]review.DiffReview, 0, len(reviews))
	for _, r := range reviews {
		comments := make([]review.DiffReviewComment, 0, len(r.GetComments()))
		for _, c := range r.GetComments() {
			comments = append(comments, review.DiffReviewComment{
				ID: c.GetId(), ReviewID: c.GetReviewId(), Kind: review.DiffCommentKind(c.GetKind()), Path: c.GetPath(),
				StartSide: review.DiffSide(c.GetStartSide()), StartLine: int(c.GetStartLine()),
				EndSide: review.DiffSide(c.GetEndSide()), EndLine: int(c.GetEndLine()),
				Quote: c.GetQuote(), Text: c.GetText(), Severity: review.Severity(c.GetSeverity()),
			})
		}
		out = append(out, review.DiffReview{
			ID: r.GetId(), SessionID: r.GetSessionId(), Author: r.GetAuthor(), Agent: r.GetAgent(), Head: r.GetHead(),
			Summary: r.GetSummary(), CreatedAt: timestampFromProto(r.GetCreatedAt()), Comments: comments,
		})
	}
	return out
}
