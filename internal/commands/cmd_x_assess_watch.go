package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/core/terminal/assess"
	"github.com/colonyops/hive/internal/core/terminal/status"
	terminaltmux "github.com/colonyops/hive/internal/core/terminal/tmux"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/pkg/iojson"
)

// paneExtraDelimiter separates pane_title from pane_in_mode in one
// display-message call; tmux replaces literal tabs in format strings, so use
// a printable delimiter (matches internal/core/terminal/tmux/pane_lister.go's
// convention).
const paneExtraDelimiter = "|||"

type assessPaneCapture interface {
	CapturePane(ctx context.Context, target string) (string, error)
}

type paneExtraFunc func(ctx context.Context, target string) (title string, inMode bool, err error)

// paneExtra fetches a tmux pane's title and copy-mode state. Read-only: like
// capture-pane, display-message -p never mutates the pane.
func paneExtra(ctx context.Context, target string) (title string, inMode bool, err error) {
	out, err := exec.CommandContext(ctx, "tmux", "display-message", "-p", "-t", target,
		"#{pane_title}"+paneExtraDelimiter+"#{pane_in_mode}").Output()
	if err != nil {
		return "", false, fmt.Errorf("tmux display-message: %w", err)
	}

	parts := strings.SplitN(strings.TrimRight(string(out), "\n"), paneExtraDelimiter, 2)
	title = parts[0]
	if len(parts) > 1 {
		inMode = parts[1] == "1"
	}
	return title, inMode, nil
}

func watchTrackerOptions(app *hive.App, intervalSet bool, interval time.Duration) (status.Options, time.Duration, error) {
	opts := trackerOptions(app)
	if !intervalSet {
		interval = opts.PollInterval
	}
	if interval <= 0 {
		return status.Options{}, 0, fmt.Errorf("--interval must be greater than zero")
	}
	opts.PollInterval = interval
	return opts, interval, nil
}

func openPrivateRecordFile(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		f, createErr := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if createErr == nil {
			return f, nil
		}
		if !os.IsExist(createErr) {
			return nil, createErr
		}
		info, err = os.Lstat(path)
		if err != nil {
			return nil, err
		}
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("record path must not be a symlink")
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("record path must be a regular file")
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return nil, err
	}
	closeOnError := func(err error) (*os.File, error) {
		_ = f.Close()
		return nil, err
	}

	openedInfo, err := f.Stat()
	if err != nil {
		return closeOnError(err)
	}
	currentInfo, err := os.Lstat(path)
	if err != nil {
		return closeOnError(err)
	}
	if currentInfo.Mode()&os.ModeSymlink != 0 || !currentInfo.Mode().IsRegular() || !os.SameFile(openedInfo, currentInfo) {
		return closeOnError(fmt.Errorf("record path changed while opening"))
	}
	if err := f.Chmod(0o600); err != nil {
		return closeOnError(fmt.Errorf("setting private record permissions: %w", err))
	}
	return f, nil
}

func runAssessWatchPoll(
	ctx context.Context,
	target string,
	requestedTool string,
	generation uint64,
	capture assessPaneCapture,
	getPaneExtra paneExtraFunc,
	now func() time.Time,
	tracker *status.Tracker,
	record io.Writer,
	output io.Writer,
	jsonl bool,
) error {
	content, err := capture.CapturePane(ctx, target)
	if err != nil {
		return fmt.Errorf("capture-pane: %w", err)
	}
	title, inMode, err := getPaneExtra(ctx, target)
	if err != nil {
		return fmt.Errorf("display-message: %w", err)
	}

	tool := requestedTool
	if tool == "" {
		tool = terminal.DetectTool(content)
	}
	timestamp := now()
	if record != nil {
		frame := assessFrame{Timestamp: timestamp, Content: content, Title: title, Tool: tool, InMode: inMode}
		if err := iojson.WriteLine(record, frame); err != nil {
			return fmt.Errorf("writing --record frame: %w", err)
		}
	}

	snap := assess.Snapshot{Content: content, Title: title, Tool: tool, InMode: inMode, Generation: generation}
	published, assessment := tracker.Observe(target, snap)
	debug, _ := tracker.DebugState(target)
	return writeAssessObservation(output, jsonl, newAssessObservation(timestamp, generation, published, assessment, debug, inMode))
}

// assessWatchCmd observes a live tmux pane through the Stage 1 engine and
// Stage 2 tracker. It builds its own private, single-goroutine Tracker —
// production status fetching is unaffected by running this alongside it.
// Capture is read-only (capture-pane, display-message); it never sends keys,
// so it is safe to run against a real session on the host.
func (cmd *ExperimentalCmd) assessWatchCmd() *cli.Command {
	var (
		flagTool     string
		flagJSONL    bool
		flagRecord   string
		flagInterval time.Duration
	)

	return &cli.Command{
		Name:      "watch",
		Usage:     "Observe a live tmux pane through the assessment engine and tracker (read-only)",
		UsageText: "hive x assess watch <pane-target> [--tool <tool>] [--jsonl] [--record frames.jsonl] [--interval 1.5s]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "tool",
				Aliases:     []string{"t"},
				Usage:       "Tool to assess as (claude, codex, ...); empty auto-detects per frame",
				Destination: &flagTool,
			},
			&cli.BoolFlag{
				Name:        "jsonl",
				Usage:       "Emit machine-readable JSON lines instead of a human summary",
				Destination: &flagJSONL,
			},
			&cli.StringFlag{
				Name:        "record",
				Usage:       "Append raw frames as JSONL to this file for later `hive x assess replay`",
				Destination: &flagRecord,
			},
			&cli.DurationFlag{
				Name:        "interval",
				Usage:       "Poll interval (default: terminal.status config, falling back to tmux.poll_interval)",
				Value:       1500 * time.Millisecond,
				Destination: &flagInterval,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			if c.Args().Len() != 1 {
				return fmt.Errorf("usage: hive x assess watch <pane-target> [--tool <tool>] [--jsonl] [--record frames.jsonl] [--interval 1.5s]")
			}
			target := c.Args().First()

			opts, interval, err := watchTrackerOptions(cmd.app, c.IsSet("interval"), flagInterval)
			if err != nil {
				return err
			}

			var recordFile *os.File
			if flagRecord != "" {
				recordFile, err = openPrivateRecordFile(flagRecord)
				if err != nil {
					return fmt.Errorf("opening --record file: %w", err)
				}
				defer func() { _ = recordFile.Close() }()
			}

			tracker := status.NewTracker(assess.NewEngine(), opts)
			capture := terminaltmux.TmuxCapture{}
			writer := c.Root().Writer
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			var generation uint64
			for {
				generation++
				if err := runAssessWatchPoll(ctx, target, flagTool, generation, capture, paneExtra, time.Now, tracker, recordFile, writer, flagJSONL); err != nil {
					return err
				}

				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-ticker.C:
				}
			}
		},
	}
}
