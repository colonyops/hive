package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/core/terminal/assess"
	"github.com/colonyops/hive/internal/core/terminal/status"
	terminaltmux "github.com/colonyops/hive/internal/core/terminal/tmux"
	"github.com/colonyops/hive/pkg/iojson"
)

// paneExtraDelimiter separates pane_title from pane_in_mode in one
// display-message call; tmux replaces literal tabs in format strings, so use
// a printable delimiter (matches internal/core/terminal/tmux/pane_lister.go's
// convention).
const paneExtraDelimiter = "|||"

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

// assessWatchCmd observes a live tmux pane through the Stage 1 engine and
// Stage 2 tracker. It builds its own private, single-goroutine Tracker —
// production status fetching is unaffected by running this alongside it.
// Capture is read-only (capture-pane, display-message); it
// never sends keys, so it's safe to run against a real session on the host.
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

			opts := trackerOptions(cmd.app)
			interval := opts.PollInterval
			if c.IsSet("interval") {
				interval = flagInterval
				opts.PollInterval = interval
			}

			var recordFile *os.File
			if flagRecord != "" {
				f, err := os.OpenFile(flagRecord, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
				if err != nil {
					return fmt.Errorf("opening --record file: %w", err)
				}
				defer func() { _ = f.Close() }()
				recordFile = f
			}

			tracker := status.NewTracker(assess.NewEngine(), opts)
			capture := terminaltmux.TmuxCapture{}
			writer := c.Root().Writer

			var generation uint64
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			for {
				content, err := capture.CapturePane(ctx, target)
				if err != nil {
					return fmt.Errorf("capture-pane: %w", err)
				}
				title, inMode, err := paneExtra(ctx, target)
				if err != nil {
					return fmt.Errorf("display-message: %w", err)
				}

				now := time.Now()
				if recordFile != nil {
					frame := assessFrame{Timestamp: now, Content: content, Title: title, InMode: inMode}
					if err := iojson.WriteLine(recordFile, frame); err != nil {
						return fmt.Errorf("writing --record frame: %w", err)
					}
				}

				generation++
				tool := flagTool
				if tool == "" {
					tool = terminal.DetectTool(content)
				}
				snap := assess.Snapshot{Content: content, Title: title, Tool: tool, InMode: inMode, Generation: generation}
				published, assessment := tracker.Observe(target, snap)
				debug, _ := tracker.DebugState(target)

				out := newAssessObservation(now, generation, published, assessment, debug, inMode)
				if err := writeAssessObservation(writer, flagJSONL, out); err != nil {
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
