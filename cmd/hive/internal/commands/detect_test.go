package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/colonyops/hive/internal/core/multiplexer"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/terminal/assess"
	"github.com/colonyops/hive/internal/core/terminal/classifier"
	"github.com/stretchr/testify/assert"
)

type fakeContentCapture struct {
	content string
	err     error
	targets []multiplexer.Target
}

func (f *fakeContentCapture) CapturePane(_ context.Context, target multiplexer.Target) (string, error) {
	f.targets = append(f.targets, target)
	return f.content, f.err
}

func TestAssessDetectedAgentPane(t *testing.T) {
	target := multiplexer.Target{Session: "work", Window: "0", Pane: "1"}
	pane := classifier.PaneInput{Target: target, PaneID: "%1", PaneTitle: "codex", InMode: false}
	capture := &fakeContentCapture{content: "› Ask anything\n"}

	state, ruleID := assessDetectedAgentPane(context.Background(), pane, "codex", capture, assess.NewEngine())
	assert.Equal(t, "idle", state)
	assert.Equal(t, "codex/bare-prompt", ruleID)
	assert.Equal(t, []multiplexer.Target{target}, capture.targets, "capture must use the already-resolved pane target")
}

func TestAssessDetectedAgentPaneCaptureFailureAndEmptyToolFallback(t *testing.T) {
	pane := classifier.PaneInput{PaneID: "%1"}

	state, ruleID := assessDetectedAgentPane(context.Background(), pane, "codex", &fakeContentCapture{err: errors.New("capture")}, assess.NewEngine())
	assert.Empty(t, state)
	assert.Empty(t, ruleID)

	state, ruleID = assessDetectedAgentPane(context.Background(), pane, "", &fakeContentCapture{content: "Continue? (y/n)"}, assess.NewEngine())
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
