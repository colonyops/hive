package app

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/domain/terminal"
	statussvc "github.com/colonyops/hive/internal/hive/status"
)

func changesFixture(input *fakeAgentInput) *ReviewService {
	code := &fakeCodeAgents{
		sessions: []session.Session{
			{ID: "s1", Name: "one", State: session.StateActive},
			{ID: "s2", Name: "two", State: session.StateActive},
		},
		panes: map[string][]statussvc.AgentPane{
			"s1": {
				{PaneID: "%1", Tool: "claude", Status: terminal.StatusReady},
				{PaneID: "%2", Tool: "claude", Status: terminal.StatusApproval},
				{PaneID: "%3", Tool: "", Status: terminal.StatusReady},
			},
			"s2": {{PaneID: "%9", Tool: "claude", Status: terminal.StatusReady}},
		},
	}
	return newReviewService(ReviewDeps{Code: code, Input: input})
}

type fakeServerAgents struct {
	sess  session.Session
	panes []statussvc.AgentPane
	err   error
	input *fakeAgentInput
	asked []string
}

func (f *fakeServerAgents) SessionAgentPanes(_ context.Context, server, id string) (session.Session, []statussvc.AgentPane, error) {
	f.asked = append(f.asked, server+":"+id)
	return f.sess, f.panes, f.err
}

func (f *fakeServerAgents) Input(context.Context, string) (agentInput, error) { return f.input, nil }

func serverChangesFixture(local *fakeAgentInput, servers *fakeServerAgents) *ReviewService {
	svc := changesFixture(local)
	svc.servers = servers
	return svc
}

func diffComment(text string) DiffCommentInput {
	return DiffCommentInput{
		Path:      "a.go",
		StartSide: "additions",
		StartLine: 4,
		EndSide:   "additions",
		EndLine:   4,
		Quote:     "+x",
		Text:      text,
	}
}

func TestChangesTargetsListsOnlyThatSessionsAgents(t *testing.T) {
	t.Parallel()
	targets, err := changesFixture(&fakeAgentInput{}).ChangesTargets(t.Context(), "s1")
	require.NoError(t, err)
	panes := make([]string, 0, len(targets))
	for _, target := range targets {
		panes = append(panes, target.Pane)
	}
	assert.Equal(t, []string{"%1", "%2"}, panes)
}

func TestSendChangesPastesIntoThePane(t *testing.T) {
	t.Parallel()
	input := &fakeAgentInput{}
	svc := changesFixture(input)
	require.NoError(t, svc.SendChanges(t.Context(), "s1", "%1", []DiffCommentInput{diffComment("rename")}))
	require.Len(t, input.sent, 1)
	assert.Equal(t, "%1", input.sent[0].pane)
	assert.Contains(t, input.sent[0].text, "a.go (line 4)")
	assert.Contains(t, input.sent[0].text, "> rename")
}

func TestSendChangesRefusals(t *testing.T) {
	t.Parallel()
	badRange := diffComment("x")
	badRange.StartLine = 0
	cases := []struct {
		name     string
		pane     string
		session  string
		comments []DiffCommentInput
		kind     Kind
	}{
		{"nothing to send", "%1", "s1", nil, KindInvalid},
		{"empty comment", "%1", "s1", []DiffCommentInput{diffComment("  ")}, KindInvalid},
		{"bad range", "%1", "s1", []DiffCommentInput{badRange}, KindInvalid},
		{"unknown kind", "%1", "s1", []DiffCommentInput{{Kind: "hunk", Path: "a.go", Text: "x"}}, KindInvalid},
		{"empty overall comment", "%1", "s1", []DiffCommentInput{{Kind: "review", Text: " "}}, KindInvalid},
		{"file comment without a file", "%1", "s1", []DiffCommentInput{{Kind: "file", Text: "x"}}, KindInvalid},
		{"agent on a prompt", "%2", "s1", []DiffCommentInput{diffComment("x")}, KindConflict},
		{"a shell pane", "%3", "s1", []DiffCommentInput{diffComment("x")}, KindNotFound},
		{"another session's pane", "%9", "s1", []DiffCommentInput{diffComment("x")}, KindNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := &fakeAgentInput{}
			err := changesFixture(input).SendChanges(t.Context(), tc.session, tc.pane, tc.comments)
			require.Error(t, err)
			assert.Equal(t, tc.kind, KindOf(err))
			assert.Empty(t, input.sent)
		})
	}
}

func TestSendChangesReportsAFailedPaste(t *testing.T) {
	t.Parallel()
	svc := changesFixture(&fakeAgentInput{err: errors.New("tmux gone")})
	err := svc.SendChanges(t.Context(), "s1", "%1", []DiffCommentInput{diffComment("x")})
	require.Error(t, err)
	assert.Equal(t, KindInternal, KindOf(err))
}

func TestChangesFeedbackFormatsWithoutTheSession(t *testing.T) {
	t.Parallel()
	text, err := newReviewService(ReviewDeps{}).ChangesFeedback(t.Context(), []DiffCommentInput{diffComment("rename")})
	require.NoError(t, err)
	assert.Contains(t, text, "a.go (line 4)")
	assert.Contains(t, text, "> rename")
}

func TestChangesFeedbackTakesFileAndOverallComments(t *testing.T) {
	t.Parallel()
	text, err := newReviewService(ReviewDeps{}).ChangesFeedback(t.Context(), []DiffCommentInput{
		{Kind: "review", Text: "looks close"},
		{Kind: "file", Path: "a.go", Text: "add tests"},
	})
	require.NoError(t, err)
	assert.Contains(t, text, "## Overall\n> looks close\n")
	assert.Contains(t, text, "## 1. a.go (whole file)\n> add tests\n")
}

func TestChangesTargetsTellApartAgentsThatReadTheSame(t *testing.T) {
	t.Parallel()
	code := &fakeCodeAgents{
		sessions: []session.Session{{ID: "s1", Name: "one", State: session.StateActive}},
		panes: map[string][]statussvc.AgentPane{"s1": {
			{PaneID: "%1", WindowName: "claude", Tool: "claude", Status: terminal.StatusReady},
			{PaneID: "%2", WindowName: "claude", Tool: "claude", Status: terminal.StatusReady},
			{PaneID: "%3", WindowName: "codex", Tool: "codex", Status: terminal.StatusReady},
		}},
	}
	targets, err := newReviewService(ReviewDeps{Code: code}).ChangesTargets(t.Context(), "s1")
	require.NoError(t, err)
	labels := make([]string, 0, len(targets))
	for _, target := range targets {
		labels = append(labels, target.Label)
	}
	assert.Equal(t, []string{"one · claude (%1)", "one · claude (%2)", "one · codex"}, labels)
}

func TestChangesTargetsOfAServerSessionComeFromTheServer(t *testing.T) {
	t.Parallel()
	servers := &fakeServerAgents{
		sess: session.Session{ID: "s1", Name: "remote", State: session.StateActive, Remote: "https://github.com/acme/site"},
		panes: []statussvc.AgentPane{
			{PaneID: "%4", WindowName: "claude", Tool: "claude", Status: terminal.StatusReady},
			{PaneID: "%5", Tool: "", Status: terminal.StatusReady},
		},
	}
	targets, err := serverChangesFixture(&fakeAgentInput{}, servers).ChangesTargets(t.Context(), "box:s1")
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, "box:s1", targets[0].ID, "the target keeps the id the Code view knows")
	assert.Equal(t, "%4", targets[0].Pane)
	assert.Equal(t, "site / remote · claude", targets[0].Label)
	assert.Equal(t, []string{"box:s1"}, servers.asked)
}

func TestSendChangesToAServerSessionTypesIntoTheServersTmux(t *testing.T) {
	t.Parallel()
	local, remote := &fakeAgentInput{}, &fakeAgentInput{}
	servers := &fakeServerAgents{
		sess:  session.Session{ID: "s1", Name: "remote", State: session.StateActive},
		panes: []statussvc.AgentPane{{PaneID: "%1", Tool: "claude", Status: terminal.StatusReady}},
		input: remote,
	}
	svc := serverChangesFixture(local, servers)

	require.NoError(t, svc.SendChanges(t.Context(), "box:s1", "%1", []DiffCommentInput{diffComment("rename")}))

	assert.Empty(t, local.sent, "pane %1 of this machine is another pane")
	require.Len(t, remote.sent, 1)
	assert.Equal(t, "%1", remote.sent[0].pane)
	assert.Contains(t, remote.sent[0].text, "rename")
}

func TestSendChangesToAServerSessionFails(t *testing.T) {
	t.Parallel()
	for name, servers := range map[string]*fakeServerAgents{
		"unreachable": {err: Errorf(KindUnavailable, "server down")},
		"recycled":    {sess: session.Session{ID: "s1", State: session.StateRecycled}, panes: []statussvc.AgentPane{{PaneID: "%1", Tool: "claude", Status: terminal.StatusReady}}},
		"on a prompt": {sess: session.Session{ID: "s1", State: session.StateActive}, panes: []statussvc.AgentPane{{PaneID: "%1", Tool: "claude", Status: terminal.StatusApproval}}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			servers.input = &fakeAgentInput{}
			err := serverChangesFixture(&fakeAgentInput{}, servers).SendChanges(t.Context(), "box:s1", "%1", []DiffCommentInput{diffComment("x")})
			require.Error(t, err)
			assert.Empty(t, servers.input.sent)
		})
	}
}
