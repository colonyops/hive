package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/core/terminal/assess"
	"github.com/colonyops/hive/internal/core/terminal/status"
)

// assessReplayCmd feeds recorded frames (from `watch --record`) through a
// fresh Engine+Tracker on a virtual clock driven by the frames' own
// timestamps, rather than wall time — the same recording always produces
// the same decision sequence, which is what makes a live bug reproducible
// offline.
func (cmd *ExperimentalCmd) assessReplayCmd() *cli.Command {
	var (
		flagTool  string
		flagJSONL bool
	)

	return &cli.Command{
		Name:      "replay",
		Usage:     "Replay recorded frames through a fresh engine+tracker on a virtual clock",
		UsageText: "hive x assess replay <frames.jsonl> [--tool <tool>] [--jsonl]",
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
		},
		Action: func(_ context.Context, c *cli.Command) error {
			if c.Args().Len() != 1 {
				return fmt.Errorf("usage: hive x assess replay <frames.jsonl> [--tool <tool>] [--jsonl]")
			}

			frames, err := readAssessFrames(c.Args().First())
			if err != nil {
				return err
			}

			opts := status.DefaultOptions()
			if cmd.app != nil && cmd.app.Config != nil {
				opts = status.OptionsFromConfig(cmd.app.Config.Terminal.Status, cmd.app.Config.Tmux.PollInterval)
			}

			var virtualNow time.Time
			opts.Clock = func() time.Time { return virtualNow }

			tracker := status.NewTracker(assess.NewEngine(), opts)

			return replayFrames(c.Root().Writer, tracker, frames, flagTool, flagJSONL, &virtualNow)
		},
	}
}

// readAssessFrames reads a JSONL file of assessFrame records, one per line.
func readAssessFrames(path string) ([]assessFrame, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening frames file: %w", err)
	}
	defer func() { _ = f.Close() }()

	var frames []assessFrame
	dec := json.NewDecoder(f)
	for dec.More() {
		var frame assessFrame
		if err := dec.Decode(&frame); err != nil {
			return nil, fmt.Errorf("parsing frame: %w", err)
		}
		frames = append(frames, frame)
	}
	return frames, nil
}

// replayFrames feeds frames through tracker, advancing virtualNow to each
// frame's recorded timestamp before observing it, and reports one
// observation line per frame in recording order. A fresh tracker plus a
// clock driven only by recorded data (never wall time) is what makes this
// deterministic across repeated runs of the same file.
func replayFrames(w io.Writer, tracker *status.Tracker, frames []assessFrame, tool string, jsonl bool, virtualNow *time.Time) error {
	var generation uint64
	for _, f := range frames {
		*virtualNow = f.Timestamp

		frameTool := tool
		if frameTool == "" {
			frameTool = terminal.DetectTool(f.Content)
		}

		generation++
		snap := assess.Snapshot{Content: f.Content, Title: f.Title, Tool: frameTool, InMode: f.InMode, Generation: generation}
		published, assessment := tracker.Observe("replay", snap)
		debug, _ := tracker.DebugState("replay")

		out := assessObservationOutput{
			Timestamp:      f.Timestamp,
			Generation:     generation,
			Assessment:     assessStateOutput{State: assessment.State, RuleID: assessment.RuleID, Hold: assessment.Hold},
			Published:      published,
			Candidate:      debug.Candidate,
			CandidatePolls: debug.CandidatePolls,
			Churned:        debug.Churned,
			InMode:         f.InMode,
		}
		if err := writeAssessObservation(w, jsonl, out); err != nil {
			return err
		}
	}
	return nil
}
