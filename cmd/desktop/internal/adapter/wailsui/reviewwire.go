package wailsui

import (
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/internal/domain/review"
)

type CommentWire struct {
	ID        string    `json:"id"`
	StartLine int       `json:"startLine"`
	EndLine   int       `json:"endLine"`
	Context   string    `json:"context"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
}

// DocumentReviewWire is app.DocumentReview with its comments converted.
type DocumentReviewWire struct {
	SessionID string        `json:"sessionId"`
	Current   bool          `json:"current"`
	Comments  []CommentWire `json:"comments"`
}

func commentWireOf(c review.Comment) CommentWire {
	return CommentWire{
		ID:        c.ID,
		StartLine: c.StartLine,
		EndLine:   c.EndLine,
		Context:   c.ContextText,
		Text:      c.CommentText,
		CreatedAt: c.CreatedAt,
	}
}

func documentReviewWireOf(r app.DocumentReview) DocumentReviewWire {
	comments := make([]CommentWire, 0, len(r.Comments))
	for _, c := range r.Comments {
		comments = append(comments, commentWireOf(c))
	}
	return DocumentReviewWire{SessionID: r.SessionID, Current: r.Current, Comments: comments}
}
