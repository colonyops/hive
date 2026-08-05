package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/urfave/cli/v3"
)

// paneDriver renders one recorded assessFrame into a real tmux pane.
type paneDriver interface {
	DriveFrame(ctx context.Context, target string, frame assessFrame) error
}

type commandRunner func(ctx context.Context, name string, args ...string) error

func runCommand(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}

// tmuxPaneDriver is the production paneDriver.
//
// Mechanism, and why: it renders a frame by replacing the pane's foreground
// process each frame via `tmux respawn-pane -k`, rather than typing the
// content in via send-keys. send-keys delivers characters to whatever
// process is already reading the pane's pty as *input* — a plain shell
// would echo them back and then try to execute the frame's text as a
// command, which is exactly wrong for content meant to be the pane's
// *output*. respawn-pane instead kills that process and starts a fresh one,
// so the frame's text lands in the pane purely as that new process's own
// stdout: no echo, no interpretation as a command. That is precisely what a
// real agent CLI's own output would look like — which is the point ("the
// pane LOOKS like a live agent to the whole pipeline").
//
// The respawned process is `sh -c 'clear; cat <file>; exec tail -f
// /dev/null'`: clear resets the screen so stale content never bleeds into
// the next frame, cat renders the frame's exact captured text, and the
// trailing tail parks the pane on that content — as opposed to exiting
// back to a shell prompt, which would add a trailing line capture-pane
// would see that was never part of the recording. `tail -f /dev/null`
// (rather than `sleep infinity`) is deliberate: GNU coreutils' sleep
// accepts "infinity" as a duration, but BSD/macOS sleep does not and exits
// immediately, killing the parked process and closing the pane out from
// under the next frame's respawn-pane. `tail -f` blocks forever on both
// GNU and BSD without relying on either sleep dialect.
//
// Known limitation: frame.InMode (tmux copy-mode) is not replicated.
// respawn-pane exits copy mode as a side effect of killing the previous
// process, and tmux has no non-interactive primitive to re-enter it against
// scripted scrollback. Only Content and Title are driven.
type tmuxPaneDriver struct {
	// framePath is a single reused scratch file (not one per frame): by the
	// time the next frame is driven, the frame interval has elapsed and the
	// previous frame's `cat` has long since finished reading it, so
	// overwriting it in place is safe and avoids per-frame temp-file
	// cleanup.
	framePath string
	run       commandRunner
}

// newTmuxPaneDriver creates the scratch file tmuxPaneDriver writes frame
// content to, and returns a cleanup func the caller must defer.
func newTmuxPaneDriver() (tmuxPaneDriver, func(), error) {
	dir, err := os.MkdirTemp("", "hive-assess-drive-")
	if err != nil {
		return tmuxPaneDriver{}, nil, fmt.Errorf("creating scratch dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	return tmuxPaneDriver{framePath: filepath.Join(dir, "frame.txt"), run: runCommand}, cleanup, nil
}

func (d tmuxPaneDriver) DriveFrame(ctx context.Context, target string, frame assessFrame) error {
	if err := os.WriteFile(d.framePath, []byte(frame.Content), 0o600); err != nil {
		return fmt.Errorf("writing frame content: %w", err)
	}

	// framePath is program-generated (os.MkdirTemp plus a fixed literal
	// filename), never derived from frame content or other user input, so a
	// single-quote wrap is sufficient quoting without a general escaper.
	runner := d.run
	if runner == nil {
		runner = runCommand
	}

	shellCmd := fmt.Sprintf("clear; cat '%s'; exec tail -f /dev/null", d.framePath)
	if err := runner(ctx, "tmux", "respawn-pane", "-k", "-t", target, shellCmd); err != nil {
		return fmt.Errorf("tmux respawn-pane: %w", err)
	}

	if err := runner(ctx, "tmux", "select-pane", "-t", target, "-T", frame.Title); err != nil {
		return fmt.Errorf("tmux select-pane (title): %w", err)
	}

	return nil
}

// ctxSleep is driveFrames' default wait: like time.Sleep, but returns early
// if ctx is canceled mid-wait.
func ctxSleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// assessDriveCmd registers `hive x assess drive`: it replays previously
// recorded frames (from `watch --record`) into a real tmux pane with their
// original relative timing, exercising the true
// capture-pane->list-panes->assess path end to end with zero agent
// credentials.
//
// Mutates the target via `tmux respawn-pane -k`/`select-pane` — see
// ensureContainerSafe: this fails closed outside `mise container` unless
// --allow-host is passed.
func (cmd *ExperimentalCmd) assessDriveCmd() *cli.Command {
	var (
		flagTarget    string
		flagAllowHost bool
	)

	return &cli.Command{
		Name:      "drive",
		Usage:     "Replay recorded frames into a real tmux pane with original timing (sends input; container-only)",
		UsageText: "hive x assess drive <frames.jsonl> --target <pane> [--allow-host]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "target",
				Usage:       "tmux pane target to drive (e.g. session:0.0)",
				Required:    true,
				Destination: &flagTarget,
			},
			&cli.BoolFlag{
				Name:        "allow-host",
				Usage:       "Bypass the host-tmux safety interlock (deliberate, eyes-open exceptions only)",
				Destination: &flagAllowHost,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			if c.Args().Len() != 1 {
				return fmt.Errorf("usage: hive x assess drive <frames.jsonl> --target <pane> [--allow-host]")
			}

			return runAssessDriveCmd(ctx, c.Root().Writer, c.Args().First(), flagTarget, flagAllowHost, resolveTmuxSocketPath, runningInContainer)
		},
	}
}

// runAssessDriveCmd is assessDriveCmd's Action, minus the cli.Command
// plumbing: resolve is injected so the safety interlock is testable without
// a real tmux server. The tmux interaction inside driveFrames is not
// exercised by this repo's own tests — it is exercised only by hand, inside
// `mise container`.
func runAssessDriveCmd(ctx context.Context, w io.Writer, framesPath, target string, allowHost bool, resolve socketPathResolver, isolated isolationDetector) error {
	if err := ensureContainerSafe(ctx, target, allowHost, resolve, isolated); err != nil {
		return err
	}

	frames, err := readAssessFrames(framesPath)
	if err != nil {
		return err
	}
	if len(frames) == 0 {
		return fmt.Errorf("no frames in %s", framesPath)
	}

	driver, cleanup, err := newTmuxPaneDriver()
	if err != nil {
		return err
	}
	defer cleanup()

	return driveFrames(ctx, w, driver, target, frames, ctxSleep)
}

// driveFrames drives each frame in order, sleeping between frames for the
// same interval that separated them when recorded (clamped to zero if the
// recording's timestamps are out of order) — this is what makes the
// replay-into-pane feel like watching the original session happen again in
// real time, not an instant dump.
func driveFrames(ctx context.Context, w io.Writer, driver paneDriver, target string, frames []assessFrame, sleep func(context.Context, time.Duration)) error {
	for i, f := range frames {
		if i > 0 {
			if delta := f.Timestamp.Sub(frames[i-1].Timestamp); delta > 0 {
				sleep(ctx, delta)
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		if err := driver.DriveFrame(ctx, target, f); err != nil {
			return fmt.Errorf("driving frame %d/%d: %w", i+1, len(frames), err)
		}

		if _, err := fmt.Fprintf(w, "%s frame=%d/%d title=%q inMode=%v\n",
			f.Timestamp.Format(time.RFC3339), i+1, len(frames), f.Title, f.InMode); err != nil {
			return err
		}
	}
	return nil
}
