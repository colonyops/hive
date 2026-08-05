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

	"github.com/colonyops/hive/internal/hive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestAssessDriveCmd_MissingArgErrors(t *testing.T) {
	flags := &Flags{}
	cmd := NewExperimentalCmd(flags, &hive.App{})

	app := &cli.Command{Name: "hive"}
	cmd.Register(app)

	err := app.Run(context.Background(), []string{"hive", "x", "assess", "drive"})
	require.Error(t, err)
}

// TestRunAssessDriveCmd_RefusesWithoutAllowHost proves the interlock runs
// before anything else in `drive`: a fake resolver reporting the host
// default socket must produce the container-only refusal, with no real
// tmux call ever made (the resolver itself is the only tmux touch-point,
// and it is faked here).
func TestRunAssessDriveCmd_RefusesWithoutAllowHost(t *testing.T) {
	resolve := func(_ context.Context, _ string) (string, error) {
		return "/tmp/tmux-501/default", nil
	}

	var buf bytes.Buffer
	err := runAssessDriveCmd(context.Background(), &buf, "/nonexistent/frames.jsonl", "mypane:0.0", false, resolve, notIsolated)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mise container")
	assert.Contains(t, err.Error(), "--allow-host")
}

// TestRunAssessDriveCmd_AllowHostNeverCallsResolver proves --allow-host
// skips socket resolution entirely (ensureContainerSafe's own contract) by
// wiring a resolver that fails the test if it is ever invoked, then letting
// the run fail harmlessly at the frames-file stage instead of ever reaching
// real tmux.
func TestRunAssessDriveCmd_AllowHostNeverCallsResolver(t *testing.T) {
	resolve := func(_ context.Context, _ string) (string, error) {
		t.Fatal("resolve must not be called when --allow-host is set")
		return "", nil
	}

	var buf bytes.Buffer
	err := runAssessDriveCmd(context.Background(), &buf, "/nonexistent/frames.jsonl", "mypane:0.0", true, resolve, notIsolated)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "opening frames file")
}

func TestRunAssessDriveCmd_EmptyFramesFileErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.jsonl")
	require.NoError(t, os.WriteFile(path, nil, 0o600))

	resolve := func(_ context.Context, _ string) (string, error) {
		t.Fatal("resolve must not be called when --allow-host is set")
		return "", nil
	}

	var buf bytes.Buffer
	err := runAssessDriveCmd(context.Background(), &buf, path, "mypane:0.0", true, resolve, notIsolated)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no frames in")
}

func TestTmuxPaneDriverClearsEmptyTitle(t *testing.T) {
	type invocation struct {
		name string
		args []string
	}
	var invocations []invocation
	driver := tmuxPaneDriver{
		framePath: filepath.Join(t.TempDir(), "frame.txt"),
		run: func(_ context.Context, name string, args ...string) error {
			invocations = append(invocations, invocation{name: name, args: append([]string(nil), args...)})
			return nil
		},
	}

	require.NoError(t, driver.DriveFrame(context.Background(), "mypane:0.0", assessFrame{Content: "frame", Title: ""}))
	require.Len(t, invocations, 2)
	assert.Equal(t, "tmux", invocations[1].name)
	assert.Equal(t, []string{"select-pane", "-t", "mypane:0.0", "-T", ""}, invocations[1].args)
}

// --- driveFrames (pure: fake driver, fake sleep, no tmux) ---

type fakePaneDriver struct {
	frames []assessFrame
	failAt int // index (0-based) of the DriveFrame call that should fail; -1 = never
}

func (d *fakePaneDriver) DriveFrame(_ context.Context, _ string, frame assessFrame) error {
	idx := len(d.frames)
	d.frames = append(d.frames, frame)
	if d.failAt == idx {
		return errors.New("boom")
	}
	return nil
}

func TestDriveFrames_DrivesInOrderWithRecordedTiming(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	frames := []assessFrame{
		{Timestamp: base, Content: "frame0", Title: "t0"},
		{Timestamp: base.Add(1500 * time.Millisecond), Content: "frame1", Title: "t1"},
		{Timestamp: base.Add(4500 * time.Millisecond), Content: "frame2", Title: "t2"},
	}

	driver := &fakePaneDriver{failAt: -1}
	var sleeps []time.Duration
	fakeSleep := func(_ context.Context, d time.Duration) { sleeps = append(sleeps, d) }

	var buf bytes.Buffer
	err := driveFrames(context.Background(), &buf, driver, "mypane:0.0", frames, fakeSleep)
	require.NoError(t, err)

	require.Len(t, driver.frames, 3)
	assert.Equal(t, "frame0", driver.frames[0].Content)
	assert.Equal(t, "frame1", driver.frames[1].Content)
	assert.Equal(t, "frame2", driver.frames[2].Content)

	require.Len(t, sleeps, 2, "one sleep between each pair of frames")
	assert.Equal(t, 1500*time.Millisecond, sleeps[0])
	assert.Equal(t, 3000*time.Millisecond, sleeps[1])

	assert.Contains(t, buf.String(), "frame=1/3")
	assert.Contains(t, buf.String(), "frame=3/3")
}

func TestDriveFrames_ClampsNonPositiveDelta(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	frames := []assessFrame{
		{Timestamp: base, Content: "frame0"},
		{Timestamp: base.Add(-time.Second), Content: "frame1"}, // out of order relative to frame0
	}

	driver := &fakePaneDriver{failAt: -1}
	var sleeps []time.Duration
	fakeSleep := func(_ context.Context, d time.Duration) { sleeps = append(sleeps, d) }

	err := driveFrames(context.Background(), io.Discard, driver, "mypane:0.0", frames, fakeSleep)
	require.NoError(t, err)
	assert.Empty(t, sleeps, "a non-positive delta must not sleep")
	assert.Len(t, driver.frames, 2)
}

func TestDriveFrames_PropagatesDriverError(t *testing.T) {
	frames := []assessFrame{{Content: "frame0"}, {Content: "frame1"}}
	driver := &fakePaneDriver{failAt: 1}
	fakeSleep := func(_ context.Context, _ time.Duration) {}

	err := driveFrames(context.Background(), io.Discard, driver, "mypane:0.0", frames, fakeSleep)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "frame 2/2")
	assert.Len(t, driver.frames, 2, "the failing frame is still recorded as attempted")
}

func TestDriveFrames_StopsOnCanceledContext(t *testing.T) {
	frames := []assessFrame{{Content: "frame0"}, {Content: "frame1"}}
	driver := &fakePaneDriver{failAt: -1}
	fakeSleep := func(_ context.Context, _ time.Duration) {}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := driveFrames(ctx, io.Discard, driver, "mypane:0.0", frames, fakeSleep)
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, driver.frames, "a canceled context must stop before driving any frame")
}
