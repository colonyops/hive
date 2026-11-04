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

// DiffReviewWire is one agent's review of a session's branch diff.
type DiffReviewWire struct {
	ID        string                  `json:"id"`
	Author    string                  `json:"author"`
	Agent     string                  `json:"agent"`
	Head      string                  `json:"head"`
	Summary   string                  `json:"summary"`
	CreatedAt time.Time               `json:"createdAt"`
	Comments  []DiffReviewCommentWire `json:"comments"`
}

// DiffReviewCommentWire is one comment. Kind is "" for rows and "file" for a
// whole file, as the frontend's own diff comments spell it.
type DiffReviewCommentWire struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	StartSide string `json:"startSide"`
	StartLine int    `json:"startLine"`
	EndSide   string `json:"endSide"`
	EndLine   int    `json:"endLine"`
	Quote     string `json:"quote"`
	Severity  string `json:"severity"`
	Text      string `json:"text"`
}

func diffReviewWireOf(r review.DiffReview) DiffReviewWire {
	comments := make([]DiffReviewCommentWire, 0, len(r.Comments))
	for _, c := range r.Comments {
		comments = append(comments, DiffReviewCommentWire{
			ID: c.ID, Kind: string(c.Kind), Path: c.Path,
			StartSide: string(c.StartSide), StartLine: c.StartLine, EndSide: string(c.EndSide), EndLine: c.EndLine,
			Quote: c.Quote, Severity: string(c.Severity), Text: c.Text,
		})
	}
	return DiffReviewWire{
		ID: r.ID, Author: r.Author, Agent: r.Agent, Head: r.Head, Summary: r.Summary,
		CreatedAt: r.CreatedAt, Comments: comments,
	}
}
