package app

import (
	"context"
	"strings"

	"github.com/colonyops/hive/internal/domain/review"
	"github.com/colonyops/hive/internal/domain/terminal"
)

// DiffCommentInput is a comment on rows of a session's diff. Unlike a Drive
// comment it stays in the frontend until it is sent: the lines it points at move
// whenever the agent edits the file, so nothing here keeps it in hive.db.
type DiffCommentInput struct {
	Path      string `json:"path"`
	StartSide string `json:"startSide"` // additions | deletions
	StartLine int    `json:"startLine"`
	EndSide   string `json:"endSide"` // additions | deletions
	EndLine   int    `json:"endLine"`
	Quote     string `json:"quote"`
	Text      string `json:"text"`
}

// ChangesTargets lists the agents of one Code session.
func (s *ReviewService) ChangesTargets(ctx context.Context, sessionID string) ([]AgentTarget, error) {
	code, err := s.codeTargets(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	targets := make([]AgentTarget, 0, len(code))
	for _, t := range code {
		targets = append(targets, t.AgentTarget)
	}
	return targets, nil
}

// ChangesFeedback formats comments on a session's changes, for the clipboard
// when they cannot be delivered. It reads nothing of the session, so it still
// works for one that has been deleted.
func (s *ReviewService) ChangesFeedback(comments []DiffCommentInput) (string, error) {
	converted, err := diffComments(comments)
	if err != nil {
		return "", err
	}
	return review.DiffFeedback(converted), nil
}

// SendChanges pastes the feedback into one of the session's agent panes and
// submits it. The frontend clears its comments only when this succeeds.
func (s *ReviewService) SendChanges(ctx context.Context, sessionID, pane string, comments []DiffCommentInput) error {
	text, err := s.ChangesFeedback(comments)
	if err != nil {
		return err
	}
	if text == "" {
		return Errorf(KindInvalid, "there is no feedback to send")
	}
	return s.deliver(ctx, AgentTargetRef{Kind: AgentTargetSession, ID: sessionID, Pane: pane}, text)
}

// deliver resolves the target again, because the pick may be minutes old, and
// refuses an agent on a prompt, because the Enter would answer it.
func (s *ReviewService) deliver(ctx context.Context, to AgentTargetRef, text string) error {
	target, err := s.resolveTarget(ctx, to)
	if err != nil {
		return err
	}
	if target.Status == string(terminal.StatusApproval) {
		return Errorf(KindConflict, "%s is waiting on a prompt", target.Label)
	}
	return Wrap(s.input.SendText(ctx, target.deliverTo, []byte(text), s.submitDelay), KindInternal, "sending the feedback to %s", target.Label)
}

func diffComments(comments []DiffCommentInput) ([]review.DiffComment, error) {
	converted := make([]review.DiffComment, 0, len(comments))
	for _, c := range comments {
		text := strings.TrimSpace(c.Text)
		switch {
		case c.Path == "":
			return nil, Errorf(KindInvalid, "a comment needs a file")
		case text == "":
			return nil, Errorf(KindInvalid, "a comment on %s needs text", c.Path)
		case c.StartLine < 1 || c.EndLine < 1:
			return nil, Errorf(KindInvalid, "lines %d-%d of %s are not a range", c.StartLine, c.EndLine, c.Path)
		case !validDiffSide(review.DiffSide(c.StartSide)) || !validDiffSide(review.DiffSide(c.EndSide)):
			return nil, Errorf(KindInvalid, "a comment on %s has an unknown diff side", c.Path)
		}
		converted = append(converted, review.DiffComment{
			Path:      c.Path,
			StartSide: review.DiffSide(c.StartSide),
			StartLine: c.StartLine,
			EndSide:   review.DiffSide(c.EndSide),
			EndLine:   c.EndLine,
			Quote:     c.Quote,
			Text:      text,
		})
	}
	return converted, nil
}

func validDiffSide(side review.DiffSide) bool {
	return side == review.DiffSideAdditions || side == review.DiffSideDeletions
}
