package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/internal/domain/review"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/domain/terminal"
	statussvc "github.com/colonyops/hive/internal/hive/status"
)

const reviewDoc = "---\ntitle: P\n---\n# Plan\n\nStep one.\nStep two.\n"

type reviewFixture struct {
	drive driveFixture
	svc   *ReviewService
	code  *fakeCodeAgents
	chats *fakeChatAgents
	input *fakeAgentInput
	bus   *events.Bus
}

func newReviewFixture(t *testing.T) reviewFixture {
	t.Helper()
	f := reviewFixture{
		drive: newDriveFixture(t),
		code:  &fakeCodeAgents{panes: map[string][]statussvc.AgentPane{}},
		chats: &fakeChatAgents{},
		input: &fakeAgentInput{},
		bus:   newTestBus(t),
	}
	f.drive.writeDoc(t, "workspaces/ws", "plans/p.md", reviewDoc)
	f.drive.writeDoc(t, "workspaces/ws", "page.html", "<p>x</p>")
	f.svc = newReviewService(ReviewDeps{
		Reviews: f.drive.harness.engine.Reviews(),
		Drive:   f.drive.svc,
		Code:    f.code,
		Chats:   f.chats,
		Input:   f.input,
		Events:  f.bus,
		Logger:  zerolog.Nop(),
	})
	f.svc.submitDelay = 0
	return f
}

func (f reviewFixture) rewrite(t *testing.T, body string) {
	t.Helper()
	full := filepath.Join(f.drive.contextDir(), "workspaces", "ws", "plans", "p.md")
	require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
}

func (f reviewFixture) add(t *testing.T, start, end int, text string) review.Comment {
	t.Helper()
	c, err := f.svc.AddComment(t.Context(), "workspaces/ws", "plans/p.md", CommentInput{StartLine: start, EndLine: end, Context: "Step one.", Text: text})
	require.NoError(t, err)
	return c
}

func TestReviewAddListUpdateDelete(t *testing.T) {
	f := newReviewFixture(t)
	ctx := t.Context()

	empty, err := f.svc.Review(ctx, "workspaces/ws", "plans/p.md")
	require.NoError(t, err)
	assert.Empty(t, empty.SessionID)
	assert.Empty(t, empty.Comments)
	assert.True(t, empty.Current, "a document with no review takes comments")

	second := f.add(t, 7, 7, "second")
	first := f.add(t, 6, 6, "  first  ")
	assert.Equal(t, "first", first.CommentText)

	got, err := f.svc.Review(ctx, "workspaces/ws", "plans/p.md")
	require.NoError(t, err)
	assert.True(t, got.Current)
	require.Len(t, got.Comments, 2)
	assert.Equal(t, first.ID, got.Comments[0].ID, "comments come back by start line")

	require.NoError(t, f.svc.UpdateComment(ctx, "workspaces/ws", "plans/p.md", second.ID, "changed"))
	require.NoError(t, f.svc.DeleteComment(ctx, "workspaces/ws", "plans/p.md", first.ID))

	got, err = f.svc.Review(ctx, "workspaces/ws", "plans/p.md")
	require.NoError(t, err)
	require.Len(t, got.Comments, 1)
	assert.Equal(t, "changed", got.Comments[0].CommentText)
}

func TestReviewPublishesEachChangeToTheComments(t *testing.T) {
	f := newReviewFixture(t)
	ctx := t.Context()
	changes := subscribeEvents[events.ReviewChanged](t, f.bus)
	space, path := "workspaces/ws", "plans/p.md"

	comment := f.add(t, 6, 6, "first")
	require.NoError(t, f.svc.UpdateComment(ctx, space, path, comment.ID, "edited"))
	require.NoError(t, f.svc.DeleteComment(ctx, space, path, comment.ID))
	f.add(t, 6, 6, "second")
	require.NoError(t, f.svc.Clear(ctx, space, path))

	want := events.ReviewChanged{Space: space, Path: path}
	assert.Equal(t, []events.ReviewChanged{want, want, want, want, want}, requireEvents(t, changes, 5))
}

func TestReviewClearWithNothingOpenPublishesNothing(t *testing.T) {
	f := newReviewFixture(t)
	changes := subscribeEvents[events.ReviewChanged](t, f.bus)

	require.NoError(t, f.svc.Clear(t.Context(), "workspaces/ws", "plans/p.md"))

	requireEvents(t, changes, 0)
}

func TestReviewKeyAndHash(t *testing.T) {
	f := newReviewFixture(t)
	f.add(t, 4, 4, "x")

	sum := sha256.Sum256([]byte(reviewDoc))
	sess, err := f.drive.harness.engine.Reviews().GetSession(t.Context(), "drive://workspaces/ws/plans/p.md")
	require.NoError(t, err)
	assert.Equal(t, hex.EncodeToString(sum[:]), sess.ContentHash)
}

func TestReviewForeignCommentIsNotFound(t *testing.T) {
	f := newReviewFixture(t)
	f.add(t, 4, 4, "x")

	err := f.svc.UpdateComment(t.Context(), "workspaces/ws", "plans/p.md", "nope", "y")
	assert.Equal(t, KindNotFound, KindOf(err))
	err = f.svc.DeleteComment(t.Context(), "workspaces/ws", "plans/p.md", "nope")
	assert.Equal(t, KindNotFound, KindOf(err))
}

func TestReviewAddCommentValidation(t *testing.T) {
	f := newReviewFixture(t)
	for _, in := range []CommentInput{
		{StartLine: 1, EndLine: 1, Text: "  "},
		{StartLine: 0, EndLine: 1, Text: "x"},
		{StartLine: 3, EndLine: 2, Text: "x"},
		{StartLine: 7, EndLine: 8, Text: "x"},
	} {
		_, err := f.svc.AddComment(t.Context(), "workspaces/ws", "plans/p.md", in)
		assert.Equal(t, KindInvalid, KindOf(err), "%+v", in)
	}
}

func TestReviewStaleAfterRewrite(t *testing.T) {
	f := newReviewFixture(t)
	ctx := t.Context()
	c := f.add(t, 6, 6, "x")

	f.rewrite(t, reviewDoc+"Step three.\n")

	got, err := f.svc.Review(ctx, "workspaces/ws", "plans/p.md")
	require.NoError(t, err)
	assert.False(t, got.Current)
	require.Len(t, got.Comments, 1)

	_, err = f.svc.AddComment(ctx, "workspaces/ws", "plans/p.md", CommentInput{StartLine: 1, EndLine: 1, Text: "y"})
	assert.Equal(t, KindConflict, KindOf(err))

	require.NoError(t, f.svc.UpdateComment(ctx, "workspaces/ws", "plans/p.md", c.ID, "still editable"))
	feedback, err := f.svc.Feedback(ctx, "workspaces/ws", "plans/p.md", "")
	require.NoError(t, err)
	assert.Contains(t, feedback, "still editable")

	require.NoError(t, f.svc.Clear(ctx, "workspaces/ws", "plans/p.md"))
	f.add(t, 8, 8, "on the new version")
}

func TestReviewEmptyStaleSessionDoesNotBlock(t *testing.T) {
	f := newReviewFixture(t)
	ctx := t.Context()
	c := f.add(t, 6, 6, "x")
	require.NoError(t, f.svc.DeleteComment(ctx, "workspaces/ws", "plans/p.md", c.ID))

	f.rewrite(t, reviewDoc+"Step three.\n")

	got, err := f.svc.Review(ctx, "workspaces/ws", "plans/p.md")
	require.NoError(t, err)
	assert.True(t, got.Current)
	assert.Empty(t, got.SessionID)
	f.add(t, 8, 8, "on the new version")
}

func TestReviewClearFreesTheSlot(t *testing.T) {
	f := newReviewFixture(t)
	ctx := t.Context()
	f.add(t, 4, 4, "x")

	require.NoError(t, f.svc.Clear(ctx, "workspaces/ws", "plans/p.md"))
	got, err := f.svc.Review(ctx, "workspaces/ws", "plans/p.md")
	require.NoError(t, err)
	assert.Empty(t, got.SessionID)

	f.add(t, 4, 4, "again")
}

func TestReviewFinalizedRowIsAbsent(t *testing.T) {
	f := newReviewFixture(t)
	ctx := t.Context()
	f.add(t, 4, 4, "x")
	reviews := f.drive.harness.engine.Reviews()
	sess, err := reviews.GetSession(ctx, "drive://workspaces/ws/plans/p.md")
	require.NoError(t, err)
	require.NoError(t, reviews.FinalizeSession(ctx, sess.ID))

	got, err := f.svc.Review(ctx, "workspaces/ws", "plans/p.md")
	require.NoError(t, err)
	assert.Empty(t, got.SessionID)

	f.add(t, 4, 4, "fresh")
	got, err = f.svc.Review(ctx, "workspaces/ws", "plans/p.md")
	require.NoError(t, err)
	require.Len(t, got.Comments, 1)
	assert.Equal(t, "fresh", got.Comments[0].CommentText)
}

func TestReviewNonMarkdownIsInvalid(t *testing.T) {
	f := newReviewFixture(t)
	_, err := f.svc.Review(t.Context(), "workspaces/ws", "page.html")
	assert.Equal(t, KindInvalid, KindOf(err))
}

func TestReviewFeedbackUsesTheAbsolutePath(t *testing.T) {
	f := newReviewFixture(t)
	f.add(t, 6, 6, "fix")

	got, err := f.svc.Feedback(t.Context(), "workspaces/ws", "plans/p.md", "note")
	require.NoError(t, err)
	full := filepath.Join(f.drive.contextDir(), "workspaces", "ws", "plans", "p.md")
	assert.True(t, strings.HasPrefix(got, "# Document Feedback\n\nDocument: "+full+"\n"), got)
	assert.Contains(t, got, "> note")
	assert.Contains(t, got, "> fix")
}

func TestReviewAgentTargets(t *testing.T) {
	f := newReviewFixture(t)
	f.code.sessions = []session.Session{
		{ID: "s1", Name: "feat", Remote: "git@github.com:acme/app.git", State: session.StateActive},
		{ID: "s2", Name: "old", State: session.StateRecycled},
	}
	f.code.panes["s1"] = []statussvc.AgentPane{
		{PaneID: "%1", WindowName: "claude", Status: terminal.StatusReady, Tool: "claude"},
		{PaneID: "%2", WindowName: "shell", Status: terminal.StatusReady},
		{PaneID: "%3", WindowName: "codex", Status: terminal.StatusMissing, Tool: "codex"},
		{PaneID: "%4", WindowName: "ask", Status: terminal.StatusQuestion, Tool: "claude"},
	}
	f.code.panes["s2"] = []statussvc.AgentPane{{PaneID: "%9", Status: terminal.StatusReady, Tool: "claude"}}
	f.chats.running = []RunningChat{
		{ID: "c1", Workspace: "ws", Name: "chat", Pane: "%20", Status: terminal.StatusActive},
		{ID: "c3", Workspace: "ws", Name: "unknown", Pane: "%21", Status: terminal.StatusMissing},
	}

	got, err := f.svc.AgentTargets(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []AgentTarget{
		{Kind: AgentTargetSession, ID: "s1", Pane: "%1", Label: "app / feat · claude", Status: "ready"},
		{Kind: AgentTargetSession, ID: "s1", Pane: "%4", Label: "app / feat · ask", Status: "approval"},
		{Kind: AgentTargetChat, ID: "c1", Label: "ws / chat", Status: "active"},
	}, got)
}

func TestReviewSend(t *testing.T) {
	f := newReviewFixture(t)
	ctx := t.Context()
	f.code.sessions = []session.Session{{ID: "s1", Name: "feat", State: session.StateActive}}
	f.code.panes["s1"] = []statussvc.AgentPane{
		{PaneID: "%1", Status: terminal.StatusReady, Tool: "claude"},
		{PaneID: "%4", Status: terminal.StatusApproval, Tool: "claude"},
	}
	f.chats.running = []RunningChat{{ID: "c1", Workspace: "ws", Name: "chat", Pane: "%20", Status: terminal.StatusReady}}

	err := f.svc.Send(ctx, "workspaces/ws", "plans/p.md", "", AgentTargetRef{Kind: AgentTargetSession, ID: "s1", Pane: "%1"})
	assert.Equal(t, KindInvalid, KindOf(err), "nothing to send")

	f.add(t, 6, 6, "fix")
	want, err := f.svc.Feedback(ctx, "workspaces/ws", "plans/p.md", "")
	require.NoError(t, err)

	err = f.svc.Send(ctx, "workspaces/ws", "plans/p.md", "", AgentTargetRef{Kind: AgentTargetSession, ID: "s1", Pane: "%7"})
	assert.Equal(t, KindNotFound, KindOf(err), "a stale pane does not resolve")
	err = f.svc.Send(ctx, "workspaces/ws", "plans/p.md", "", AgentTargetRef{Kind: AgentTargetSession, ID: "s1", Pane: "%4"})
	assert.Equal(t, KindConflict, KindOf(err), "an agent on a prompt cannot take text")

	f.input.err = errors.New("pane gone")
	err = f.svc.Send(ctx, "workspaces/ws", "plans/p.md", "", AgentTargetRef{Kind: AgentTargetSession, ID: "s1", Pane: "%1"})
	require.Error(t, err)
	kept, err := f.svc.Review(ctx, "workspaces/ws", "plans/p.md")
	require.NoError(t, err)
	assert.Len(t, kept.Comments, 1, "a failed send keeps the review")

	f.input.err = nil
	require.NoError(t, f.svc.Send(ctx, "workspaces/ws", "plans/p.md", "", AgentTargetRef{Kind: AgentTargetChat, ID: "c1"}))
	require.Len(t, f.input.sent, 2)
	assert.Equal(t, sentText{pane: "%20", text: want}, f.input.sent[1])
	cleared, err := f.svc.Review(ctx, "workspaces/ws", "plans/p.md")
	require.NoError(t, err)
	assert.Empty(t, cleared.SessionID)
}

type fakeCodeAgents struct {
	sessions []session.Session
	panes    map[string][]statussvc.AgentPane
}

func (f *fakeCodeAgents) ListSessions(context.Context) ([]session.Session, error) {
	return f.sessions, nil
}

func (f *fakeCodeAgents) AgentPanes(_ context.Context, sessions []*session.Session) map[string][]statussvc.AgentPane {
	out := map[string][]statussvc.AgentPane{}
	for _, sess := range sessions {
		out[sess.ID] = f.panes[sess.ID]
	}
	return out
}

type fakeChatAgents struct {
	running []RunningChat
}

func (f *fakeChatAgents) RunningChats(context.Context) ([]RunningChat, error) { return f.running, nil }

type sentText struct {
	pane string
	text string
}

type fakeAgentInput struct {
	sent []sentText
	err  error
}

func (f *fakeAgentInput) SendText(_ context.Context, pane string, text []byte, _ time.Duration) error {
	f.sent = append(f.sent, sentText{pane: pane, text: string(text)})
	return f.err
}
