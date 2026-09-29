package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/terminal/assess"
	"github.com/colonyops/hive/internal/core/terminal/classifier"
	"github.com/stretchr/testify/assert"
)

func TestAssessDetectedAgentPane(t *testing.T) {
	pane := classifier.PaneInput{PaneID: "%1", PaneTitle: "codex", InMode: false}
	capture := &fakeAssessCapture{contents: []string{"› Ask anything\n"}}

	state, ruleID := assessDetectedAgentPane(context.Background(), pane, "codex", capture, assess.NewEngine())
	assert.Equal(t, "idle", state)
	assert.Equal(t, "codex/bare-prompt", ruleID)
}

func TestAssessDetectedAgentPaneCaptureFailureAndEmptyToolFallback(t *testing.T) {
	pane := classifier.PaneInput{PaneID: "%1"}

	state, ruleID := assessDetectedAgentPane(context.Background(), pane, "codex", &fakeAssessCapture{err: errors.New("capture")}, assess.NewEngine())
	assert.Empty(t, state)
	assert.Empty(t, ruleID)

	state, ruleID = assessDetectedAgentPane(context.Background(), pane, "", &fakeAssessCapture{contents: []string{"Continue? (y/n)"}}, assess.NewEngine())
	assert.Equal(t, "approval", state)
	assert.NotEmpty(t, ruleID)
}

func TestDetectTmuxSessionNames(t *testing.T) {
	sess := session.Session{
		Name: "Display Name",
		Slug: "display-name",
		Metadata: map[string]string{
			session.MetaTmuxSession: "tmux-display",
		},
	}

	got := detectTmuxSessionNames(sess)

	assert.True(t, got["tmux-display"])
	assert.True(t, got["display-name"])
	assert.True(t, got["Display Name"])
	assert.Len(t, got, 3)
}

func TestDetectTmuxSessionNames_Dedupes(t *testing.T) {
	sess := session.Session{
		Name: "same",
		Slug: "same",
		Metadata: map[string]string{
			session.MetaTmuxSession: "same",
		},
	}

	got := detectTmuxSessionNames(sess)

	assert.Equal(t, map[string]bool{"same": true}, got)
}
