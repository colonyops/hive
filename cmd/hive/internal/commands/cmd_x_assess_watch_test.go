package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/terminal/assess"
	"github.com/colonyops/hive/internal/core/terminal/status"
	"github.com/colonyops/hive/internal/hive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

type fakeAssessCapture struct {
	contents []string
	err      error
	calls    int
}

func (f *fakeAssessCapture) CapturePane(_ context.Context, _ string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	content := f.contents[f.calls]
	f.calls++
	return content, nil
}

func TestAssessWatchCmdRejectsNonPositiveIntervalBeforeCapture(t *testing.T) {
	cmd := NewExperimentalCmd(&Flags{}, &hive.App{})
	app := &cli.Command{Name: "hive", Writer: io.Discard}
	cmd.Register(app)

	err := app.Run(context.Background(), []string{"hive", "x", "assess", "watch", "%1", "--interval", "0s"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "greater than zero")
}

func TestWatchTrackerOptionsRejectsNonPositiveIntervals(t *testing.T) {
	for _, interval := range []time.Duration{0, -time.Second} {
		_, _, err := watchTrackerOptions(nil, true, interval)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "greater than zero")
	}
}

func TestTrackerOptionsUseApplicationConfig(t *testing.T) {
	stable := false
	app := &hive.App{Config: &config.Config{
		Tmux: config.TmuxConfig{PollInterval: 275 * time.Millisecond},
		Terminal: config.TerminalConfig{Status: config.TerminalStatusConfig{Confirm: config.TerminalConfirmConfig{
			Idle: config.ConfirmPolicyConfig{Polls: 7, MinDuration: 3 * time.Second, StableContent: &stable},
		}}},
	}}

	opts := trackerOptions(app)
	assert.Equal(t, 275*time.Millisecond, opts.PollInterval)
	assert.Equal(t, 7, opts.ConfirmIdle.Polls)
	assert.Equal(t, 3*time.Second, opts.ConfirmIdle.MinDuration)
	assert.False(t, opts.ConfirmIdle.StableContent)

	scenarioOpts, interval, err := scenarioTrackerOptions(app, true, 40*time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, 40*time.Millisecond, interval)
	assert.Equal(t, interval, scenarioOpts.PollInterval)
}

func TestOpenPrivateRecordFileCreatesAndHardensRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "frames.jsonl")
	f, err := openPrivateRecordFile(path)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	require.NoError(t, os.Chmod(path, 0o644))
	f, err = openPrivateRecordFile(path)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	info, err = os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestOpenPrivateRecordFileRejectsSymlinkAndNonRegularFile(t *testing.T) {
	dir := t.TempDir()
	_, err := openPrivateRecordFile(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "regular file")

	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not generally available to unprivileged Windows tests")
	}
	target := filepath.Join(dir, "target")
	require.NoError(t, os.WriteFile(target, nil, 0o600))
	link := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink(target, link))
	_, err = openPrivateRecordFile(link)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
}

func TestRunAssessWatchPollRecordsResolvedToolAndObservation(t *testing.T) {
	capture := &fakeAssessCapture{contents: []string{"codex\n› ready"}}
	getExtra := func(_ context.Context, _ string) (string, bool, error) {
		return "codex pane", false, nil
	}
	now := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	opts := status.DefaultOptions()
	opts.Clock = func() time.Time { return now }
	tracker := status.NewTracker(assess.NewEngine(), opts)
	var record, output bytes.Buffer

	err := runAssessWatchPoll(context.Background(), "%1", "", 1, capture, getExtra, func() time.Time { return now }, tracker, &record, &output, true)
	require.NoError(t, err)

	var frame assessFrame
	require.NoError(t, json.NewDecoder(&record).Decode(&frame))
	assert.Equal(t, "codex", frame.Tool)
	assert.Equal(t, "codex pane", frame.Title)
	assert.Equal(t, now, frame.Timestamp)

	var observation assessObservationOutput
	require.NoError(t, json.NewDecoder(&output).Decode(&observation))
	assert.Equal(t, uint64(1), observation.Generation)
	assert.Equal(t, now, observation.Timestamp)
}

func TestRunAssessWatchPollPropagatesCaptureAndRecordErrors(t *testing.T) {
	sentinel := errors.New("capture failed")
	tracker := status.NewTracker(assess.NewEngine(), status.DefaultOptions())
	getExtra := func(_ context.Context, _ string) (string, bool, error) { return "", false, nil }

	err := runAssessWatchPoll(context.Background(), "%1", "", 1, &fakeAssessCapture{err: sentinel}, getExtra, time.Now, tracker, nil, io.Discard, true)
	require.ErrorIs(t, err, sentinel)

	capture := &fakeAssessCapture{contents: []string{"content"}}
	err = runAssessWatchPoll(context.Background(), "%1", "agent", 1, capture, getExtra, time.Now, tracker, errWriter{}, io.Discard, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing --record frame")
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }
