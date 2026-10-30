package app

import (
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
	targets, err := changesFixture(&fakeAgentInput{}).ChangesTargets(t.Context(), "s1")
	require.NoError(t, err)
	panes := make([]string, 0, len(targets))
	for _, target := range targets {
		panes = append(panes, target.Pane)
	}
	assert.Equal(t, []string{"%1", "%2"}, panes)
}

func TestSendChangesPastesIntoThePane(t *testing.T) {
	input := &fakeAgentInput{}
	svc := changesFixture(input)
	require.NoError(t, svc.SendChanges(t.Context(), "s1", "%1", []DiffCommentInput{diffComment("rename")}))
	require.Len(t, input.sent, 1)
	assert.Equal(t, "%1", input.sent[0].pane)
	assert.Contains(t, input.sent[0].text, "a.go (line 4)")
	assert.Contains(t, input.sent[0].text, "> rename")
}

func TestSendChangesRefusals(t *testing.T) {
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
	svc := changesFixture(&fakeAgentInput{err: errors.New("tmux gone")})
	err := svc.SendChanges(t.Context(), "s1", "%1", []DiffCommentInput{diffComment("x")})
	require.Error(t, err)
	assert.Equal(t, KindInternal, KindOf(err))
}

func TestChangesTargetsTellApartAgentsThatReadTheSame(t *testing.T) {
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
