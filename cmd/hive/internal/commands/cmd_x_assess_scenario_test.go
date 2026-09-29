package commands

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/core/terminal/assess"
	"github.com/colonyops/hive/internal/core/terminal/status"
	"github.com/colonyops/hive/internal/hive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestAssessScenarioCmd_MissingArgErrors(t *testing.T) {
	flags := &Flags{}
	cmd := NewExperimentalCmd(flags, &hive.App{})

	app := &cli.Command{Name: "hive"}
	cmd.Register(app)

	err := app.Run(context.Background(), []string{"hive", "x", "assess", "scenario"})
	require.Error(t, err)
}

// TestRunAssessScenarioCmd_RefusesWithoutAllowHost proves the interlock runs
// before the scenario file is even read: a fake resolver reporting the host
// default socket must produce the container-only refusal, with no real
// tmux call ever made.
func TestRunAssessScenarioCmd_RefusesWithoutAllowHost(t *testing.T) {
	resolve := func(_ context.Context, _ string) (string, error) {
		return "/tmp/tmux-501/default", nil
	}

	var buf bytes.Buffer
	err := runAssessScenarioCmd(context.Background(), &buf, "/nonexistent/scenario.yaml", "mypane:0.0", false, false, 0, nil, resolve, notIsolated)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mise container")
	assert.Contains(t, err.Error(), "--allow-host")
}

// TestRunAssessScenarioCmd_AllowHostNeverCallsResolver proves --allow-host
// skips socket resolution entirely, then lets the run fail harmlessly at the
// scenario-file stage instead of ever reaching real tmux.
func TestRunAssessScenarioCmd_AllowHostNeverCallsResolver(t *testing.T) {
	resolve := func(_ context.Context, _ string) (string, error) {
		t.Fatal("resolve must not be called when --allow-host is set")
		return "", nil
	}

	var buf bytes.Buffer
	err := runAssessScenarioCmd(context.Background(), &buf, "/nonexistent/scenario.yaml", "mypane:0.0", true, false, 0, nil, resolve, notIsolated)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading scenario file")
}

// TestRunAssessScenarioCmd_MalformedScenarioErrorsBeforeAnyTmuxCall proves a
// scenario parse error surfaces before runScenario is ever reached (and
// therefore before any real tmux call), by using a resolver and a scenario
// file that would both explode if touched past the parse step.
func TestRunAssessScenarioCmd_MalformedScenarioErrorsBeforeAnyTmuxCall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	require.NoError(t, os.WriteFile(path, []byte("scenario: bad\nsteps:\n  - frobnicate: true\n"), 0o600))

	resolve := func(_ context.Context, _ string) (string, error) {
		t.Fatal("resolve must not be called when --allow-host is set")
		return "", nil
	}

	var buf bytes.Buffer
	err := runAssessScenarioCmd(context.Background(), &buf, path, "mypane:0.0", true, false, 0, nil, resolve, notIsolated)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown step kind")
}

func TestAssessScenarioCmdRejectsExplicitNonPositiveInterval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scenario.yaml")
	require.NoError(t, os.WriteFile(path, []byte("scenario: interval validation\nsteps:\n  - send: hello\n"), 0o600))

	for _, interval := range []string{"0s", "-1s"} {
		t.Run(interval, func(t *testing.T) {
			cmd := NewExperimentalCmd(&Flags{}, &hive.App{})
			app := &cli.Command{Name: "hive", Writer: io.Discard}
			cmd.Register(app)

			err := app.Run(context.Background(), []string{"hive", "x", "assess", "scenario", path, "--target", "%1", "--allow-host", "--interval", interval})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "--interval must be greater than zero")
		})
	}
}

func TestScenarioTrackerOptionsDefaultsOnlyWhenIntervalUnset(t *testing.T) {
	app := &hive.App{Config: &config.Config{Tmux: config.TmuxConfig{PollInterval: 275 * time.Millisecond}}}

	opts, interval, err := scenarioTrackerOptions(app, false, 0)
	require.NoError(t, err)
	assert.Equal(t, 275*time.Millisecond, interval)
	assert.Equal(t, interval, opts.PollInterval)

	for _, explicit := range []time.Duration{0, -time.Second} {
		_, _, err := scenarioTrackerOptions(app, true, explicit)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--interval must be greater than zero")
	}
}

type fakeScenarioSender struct {
	events  []string
	sendErr error
	keyErr  error
}

func (f *fakeScenarioSender) SendText(_ context.Context, _ string, text string) error {
	f.events = append(f.events, "send:"+text)
	return f.sendErr
}

func (f *fakeScenarioSender) SendKey(_ context.Context, _ string, key string) error {
	f.events = append(f.events, "key:"+key)
	return f.keyErr
}

func TestRunScenarioExecutesSendKeyExpectationAndHold(t *testing.T) {
	spec := &scenarioSpec{
		Name: "happy",
		Tool: "generic",
		Steps: []scenarioStep{
			{Kind: scenarioStepSend, Send: "deploy"},
			{Kind: scenarioStepKey, Key: "Down"},
			{Kind: scenarioStepExpect, Expect: scenarioExpect{State: terminal.StatusApproval, WithinPolls: 2, HoldsForPolls: 2}},
		},
	}
	sender := &fakeScenarioSender{}
	capture := &fakeAssessCapture{contents: []string{"Continue? (y/n)", "Continue? (y/n)"}}
	getExtra := func(_ context.Context, _ string) (string, bool, error) { return "pane", false, nil }
	tracker := status.NewTracker(assess.NewEngine(), status.DefaultOptions())
	waitCalls := 0
	wait := func() error { waitCalls++; return nil }

	polls, err := runScenario(context.Background(), "%1", spec, sender, capture, getExtra, tracker, wait)
	require.NoError(t, err)
	assert.Equal(t, []string{"send:deploy", "key:Down"}, sender.events)
	assert.Equal(t, 2, waitCalls)
	require.Len(t, polls, 2)
	assert.Equal(t, terminal.StatusApproval, polls[0].Published)
	assert.Equal(t, terminal.StatusApproval, polls[1].Published)
	assert.Equal(t, 2, polls[0].StepIndex)
}

func TestRunScenarioPropagatesSendKeyCaptureAndCancellationErrors(t *testing.T) {
	sentinel := errors.New("boom")
	getExtra := func(_ context.Context, _ string) (string, bool, error) { return "", false, nil }
	newTracker := func() *status.Tracker { return status.NewTracker(assess.NewEngine(), status.DefaultOptions()) }

	t.Run("send", func(t *testing.T) {
		spec := &scenarioSpec{Steps: []scenarioStep{{Kind: scenarioStepSend, Send: "text"}}}
		_, err := runScenario(context.Background(), "%1", spec, &fakeScenarioSender{sendErr: sentinel}, &fakeAssessCapture{}, getExtra, newTracker(), func() error { return nil })
		require.ErrorIs(t, err, sentinel)
		assert.Contains(t, err.Error(), "step 0 (send)")
	})

	t.Run("key", func(t *testing.T) {
		spec := &scenarioSpec{Steps: []scenarioStep{{Kind: scenarioStepKey, Key: "Enter"}}}
		_, err := runScenario(context.Background(), "%1", spec, &fakeScenarioSender{keyErr: sentinel}, &fakeAssessCapture{}, getExtra, newTracker(), func() error { return nil })
		require.ErrorIs(t, err, sentinel)
		assert.Contains(t, err.Error(), "step 0 (key)")
	})

	t.Run("capture", func(t *testing.T) {
		spec := &scenarioSpec{Steps: []scenarioStep{{Kind: scenarioStepExpect, Expect: scenarioExpect{State: terminal.StatusReady, WithinPolls: 1, HoldsForPolls: 1}}}}
		_, err := runScenario(context.Background(), "%1", spec, &fakeScenarioSender{}, &fakeAssessCapture{err: sentinel}, getExtra, newTracker(), func() error { return nil })
		require.ErrorIs(t, err, sentinel)
		assert.Contains(t, err.Error(), "step 0 (expect)")
	})

	t.Run("cancellation", func(t *testing.T) {
		spec := &scenarioSpec{Steps: []scenarioStep{{Kind: scenarioStepExpect, Expect: scenarioExpect{State: terminal.StatusReady, WithinPolls: 1, HoldsForPolls: 1}}}}
		_, err := runScenario(context.Background(), "%1", spec, &fakeScenarioSender{}, &fakeAssessCapture{}, getExtra, newTracker(), func() error { return context.Canceled })
		require.ErrorIs(t, err, context.Canceled)
		assert.Contains(t, err.Error(), "step 0 (expect)")
	})
}
