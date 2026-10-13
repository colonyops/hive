package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/domain/review"
	"github.com/colonyops/hive/internal/domain/terminal"
	"github.com/colonyops/hive/pkg/logutils"
)

// sendSubmitDelay is how long Send waits between the paste and the Enter, the
// same pause agent-send uses: an agent that is still drawing the paste can
// drop a keystroke that arrives with it.
const sendSubmitDelay = 500 * time.Millisecond

// DocumentReview is the open review of one Drive document.
type DocumentReview struct {
	SessionID string `json:"sessionId"` // "" until the first comment
	// Current is false when the comments belong to an earlier version of the
	// document.
	Current  bool             `json:"current"`
	Comments []review.Comment `json:"-"` // by StartLine; the adapter converts
}

// CommentInput is a new comment on a source line range.
type CommentInput struct {
	StartLine int    `json:"startLine"` // 1-based source line, front matter included
	EndLine   int    `json:"endLine"`   // inclusive
	Context   string `json:"context"`
	Text      string `json:"text"`
}

type agentInput interface {
	SendText(ctx context.Context, pane string, text []byte, submitDelay time.Duration) error
	FirstPane(ctx context.Context, session string) (string, error)
}

type chatAgents interface {
	AllSessions(ctx context.Context) ([]SessionView, error)
	SessionActivity(ctx context.Context, dir string) ([]SessionActivityItem, error)
}

// ReviewDeps builds a ReviewService.
type ReviewDeps struct {
	Reviews review.Store
	Drive   *DriveService
	Code    codeAgents
	Chats   chatAgents
	Input   agentInput
	Logger  zerolog.Logger
}

// ReviewService keeps the comments a user leaves on Drive Markdown documents
// and delivers them as feedback. Reviews live in hive.db under a drive://
// key, apart from the TUI's: the TUI anchors comments to rendered lines, and
// these are source lines.
//
// A review is never finalized. Send and Clear delete it, so the user can
// comment again on unchanged content, and a finalized row under the key is
// treated as absent. Stale reviews are never cleaned up: an agent may rewrite
// a plan while the user reviews it, and the comments must survive that.
type ReviewService struct {
	reviews     review.Store
	drive       *DriveService
	code        codeAgents
	chats       chatAgents
	input       agentInput
	submitDelay time.Duration
	log         zerolog.Logger
}

func newReviewService(d ReviewDeps) *ReviewService {
	return &ReviewService{
		reviews:     d.Reviews,
		drive:       d.Drive,
		code:        d.Code,
		chats:       d.Chats,
		input:       d.Input,
		submitDelay: sendSubmitDelay,
		log:         logutils.Component(d.Logger, "review"),
	}
}

type reviewedDocument struct {
	key    string
	hash   string
	source string
	review DocumentReview
}

// Review loads the open review of a document: the session for its current
// content, else the newest unfinalized session for an older version.
func (s *ReviewService) Review(ctx context.Context, space, path string) (DocumentReview, error) {
	doc, err := s.load(ctx, space, path)
	if err != nil {
		return DocumentReview{}, err
	}
	return doc.review, nil
}

// AddComment creates the session for the current content on first use. It
// is KindConflict while the review is on an earlier version.
func (s *ReviewService) AddComment(ctx context.Context, space, path string, in CommentInput) (review.Comment, error) {
	doc, err := s.load(ctx, space, path)
	if err != nil {
		return review.Comment{}, err
	}
	text := strings.TrimSpace(in.Text)
	switch {
	case text == "":
		return review.Comment{}, Errorf(KindInvalid, "a comment needs text")
	case in.StartLine < 1 || in.EndLine < in.StartLine:
		return review.Comment{}, Errorf(KindInvalid, "lines %d-%d are not a range", in.StartLine, in.EndLine)
	case in.EndLine > lineCount(doc.source):
		return review.Comment{}, Errorf(KindInvalid, "line %d is past the end of %s", in.EndLine, path)
	}
	if doc.review.SessionID != "" && !doc.review.Current {
		return review.Comment{}, Errorf(KindConflict, "the comments on %s are on an earlier version; send, copy or discard them first", path)
	}

	sessionID := doc.review.SessionID
	if sessionID == "" {
		sessionID, err = s.createSession(ctx, doc.key, doc.hash)
		if err != nil {
			return review.Comment{}, err
		}
	}
	comment := review.Comment{
		ID:          uuid.NewString(),
		SessionID:   sessionID,
		StartLine:   in.StartLine,
		EndLine:     in.EndLine,
		ContextText: strings.TrimSpace(in.Context),
		CommentText: text,
		CreatedAt:   time.Now(),
	}
	if err := s.reviews.SaveComment(ctx, comment); err != nil {
		return review.Comment{}, Wrap(err, KindInternal, "saving the comment on %s", path)
	}
	return comment, nil
}

// createSession re-reads on failure: the unique (path, hash) key refuses a
// second session, and a finalized one under the key is cleared away.
func (s *ReviewService) createSession(ctx context.Context, key, hash string) (string, error) {
	created, err := s.reviews.CreateSession(ctx, key, hash)
	if err == nil {
		return created.ID, nil
	}
	existing, getErr := s.reviews.GetSessionByHash(ctx, key, hash)
	if getErr != nil {
		return "", Wrap(err, KindInternal, "starting the review of %s", key)
	}
	if !existing.IsFinalized() {
		return existing.ID, nil
	}
	if err := s.reviews.DeleteSession(ctx, existing.ID); err != nil {
		return "", Wrap(err, KindInternal, "starting the review of %s", key)
	}
	created, err = s.reviews.CreateSession(ctx, key, hash)
	if err != nil {
		return "", Wrap(err, KindInternal, "starting the review of %s", key)
	}
	return created.ID, nil
}

func (s *ReviewService) UpdateComment(ctx context.Context, space, path, id, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return Errorf(KindInvalid, "a comment needs text")
	}
	comment, err := s.comment(ctx, space, path, id)
	if err != nil {
		return err
	}
	comment.CommentText = text
	return Wrap(s.reviews.UpdateComment(ctx, comment), KindInternal, "updating the comment on %s", path)
}

func (s *ReviewService) DeleteComment(ctx context.Context, space, path, id string) error {
	if _, err := s.comment(ctx, space, path, id); err != nil {
		return err
	}
	return Wrap(s.reviews.DeleteComment(ctx, id), KindInternal, "deleting the comment on %s", path)
}

// Feedback formats the open review with the document's absolute path, so any
// agent can read the file whatever its working directory.
func (s *ReviewService) Feedback(ctx context.Context, space, path, note string) (string, error) {
	doc, err := s.load(ctx, space, path)
	if err != nil {
		return "", err
	}
	return s.feedback(ctx, space, path, doc, note)
}

func (s *ReviewService) feedback(ctx context.Context, space, path string, doc reviewedDocument, note string) (string, error) {
	full, err := s.drive.Locate(ctx, space, path)
	if err != nil {
		return "", err
	}
	return review.Feedback(full, doc.review.Comments, strings.TrimSpace(note)), nil
}

// Clear deletes the open review and its comments.
func (s *ReviewService) Clear(ctx context.Context, space, path string) error {
	doc, err := s.load(ctx, space, path)
	if err != nil {
		return err
	}
	if doc.review.SessionID == "" {
		return nil
	}
	return Wrap(s.reviews.DeleteSession(ctx, doc.review.SessionID), KindInternal, "clearing the review of %s", path)
}

// Send delivers the open review's feedback to one agent and clears the
// review. The review stays when delivery fails.
func (s *ReviewService) Send(ctx context.Context, space, path, note string, to AgentTargetRef) error {
	doc, err := s.load(ctx, space, path)
	if err != nil {
		return err
	}
	text, err := s.feedback(ctx, space, path, doc, note)
	if err != nil {
		return err
	}
	if text == "" {
		return Errorf(KindInvalid, "there is no feedback to send")
	}
	target, err := s.resolveTarget(ctx, to)
	if err != nil {
		return err
	}
	if target.Status == string(terminal.StatusApproval) {
		return Errorf(KindConflict, "%s is waiting on a prompt", target.Label)
	}
	if err := s.input.SendText(ctx, target.deliverTo, []byte(text), s.submitDelay); err != nil {
		return Wrap(err, KindInternal, "sending the feedback to %s", target.Label)
	}
	if doc.review.SessionID == "" {
		return nil
	}
	return Wrap(s.reviews.DeleteSession(ctx, doc.review.SessionID), KindInternal, "the feedback was sent, but clearing the review of %s failed", path)
}

func (s *ReviewService) comment(ctx context.Context, space, path, id string) (review.Comment, error) {
	doc, err := s.load(ctx, space, path)
	if err != nil {
		return review.Comment{}, err
	}
	for _, c := range doc.review.Comments {
		if c.ID == id {
			return c, nil
		}
	}
	return review.Comment{}, Errorf(KindNotFound, "no comment %q on %s", id, path)
}

func (s *ReviewService) load(ctx context.Context, space, path string) (reviewedDocument, error) {
	if s.reviews == nil {
		return reviewedDocument{}, Errorf(KindUnavailable, "reviews are unavailable")
	}
	view, err := s.drive.Document(ctx, space, path)
	if err != nil {
		return reviewedDocument{}, err
	}
	if view.Entry.Kind != drive.KindMarkdown {
		return reviewedDocument{}, Errorf(KindInvalid, "only Markdown documents take comments")
	}
	sum := sha256.Sum256([]byte(view.Markdown))
	doc := reviewedDocument{
		key:    reviewKey(view.Space.Key, path),
		hash:   hex.EncodeToString(sum[:]),
		source: view.Markdown,
		review: DocumentReview{Current: true},
	}

	sess, err := s.openSession(ctx, doc.key, doc.hash)
	if err != nil {
		return reviewedDocument{}, err
	}
	if sess == nil {
		return doc, nil
	}
	comments, err := s.reviews.ListComments(ctx, sess.ID)
	if err != nil {
		return reviewedDocument{}, Wrap(err, KindInternal, "reading the comments on %s", path)
	}
	doc.review = DocumentReview{SessionID: sess.ID, Current: sess.ContentHash == doc.hash, Comments: comments}
	return doc, nil
}

// openSession is the session for hash, else the newest one under key. A
// finalized session is no session.
func (s *ReviewService) openSession(ctx context.Context, key, hash string) (*review.Session, error) {
	sess, err := s.reviews.GetSessionByHash(ctx, key, hash)
	if errors.Is(err, review.ErrSessionNotFound) || (err == nil && sess.IsFinalized()) {
		sess, err = s.reviews.GetSession(ctx, key)
	}
	if errors.Is(err, review.ErrSessionNotFound) || (err == nil && sess.IsFinalized()) {
		return nil, nil
	}
	if err != nil {
		return nil, Wrap(err, KindInternal, "reading the review of %s", key)
	}
	return &sess, nil
}

func reviewKey(space, path string) string {
	return "drive://" + space + "/" + path
}

func lineCount(source string) int {
	if source == "" {
		return 0
	}
	n := strings.Count(source, "\n")
	if !strings.HasSuffix(source, "\n") {
		n++
	}
	return n
}
