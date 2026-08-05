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
	"github.com/colonyops/hive/internal/hive"
)

// trackerOptions resolves the debounce options the watch/replay/scenario
// commands host their private trackers with: config-derived when the app has
// one, library defaults otherwise (e.g. running outside a hive workspace).
func trackerOptions(app *hive.App) status.Options {
	if app != nil && app.Config != nil {
		return status.OptionsFromConfig(app.Config.Terminal.Status, app.Config.Tmux.PollInterval)
	}
	return status.DefaultOptions()
}

// assessRegionsOutput mirrors assess.RegionDump for JSON output — a
// dedicated type keeps the command's wire format independent of the
// package's internal field naming.
type assessRegionsOutput struct {
	AboveBox      string `json:"aboveBox"`
	PromptBoxBody string `json:"promptBoxBody"`
	BottomLines   string `json:"bottomLines"`
	AfterLastRule string `json:"afterLastRule"`
}

type assessFileOutput struct {
	Tool    string              `json:"tool"`
	State   assess.State        `json:"state"`
	Hold    bool                `json:"hold"`
	RuleID  string              `json:"ruleID"`
	Signals []assess.Signal     `json:"signals"`
	Regions assessRegionsOutput `json:"regions"`
}

// assessCmd registers the "hive x assess" command group: "file" is a
// one-shot rule-authoring aid; "watch" and "replay" are status.Tracker
// debug tooling; "drive" and "scenario" are the calibration harness (both
// send real input to tmux and refuse to run outside a container — see
// ensureContainerSafe).
func (cmd *ExperimentalCmd) assessCmd() *cli.Command {
	return &cli.Command{
		Name:  "assess",
		Usage: "Status assessment engine debug tooling (rule-authoring aid)",
		Commands: []*cli.Command{
			cmd.assessFileCmd(),
			cmd.assessWatchCmd(),
			cmd.assessReplayCmd(),
			cmd.assessDriveCmd(),
			cmd.assessScenarioCmd(),
		},
	}
}

// assessFrame is one raw observation recorded by `watch --record` and
// consumed by replay and drive.
type assessFrame struct {
	Timestamp time.Time `json:"ts"`
	Content   string    `json:"content"`
	Title     string    `json:"title"`
	Tool      string    `json:"tool,omitempty"`
	InMode    bool      `json:"inMode"`
}

// assessObservationOutput is the shape `watch` and `replay` both emit, one
// per poll/frame.
type assessObservationOutput struct {
	Timestamp      time.Time         `json:"ts"`
	Generation     uint64            `json:"generation"`
	Assessment     assessStateOutput `json:"assessment"`
	Published      terminal.Status   `json:"published"`
	Candidate      terminal.Status   `json:"candidate"`
	CandidatePolls int               `json:"candidatePolls"`
	Churned        bool              `json:"churned"`
	InMode         bool              `json:"inMode"`
}

// assessStateOutput is the Assessment sub-object of assessObservationOutput.
type assessStateOutput struct {
	State  assess.State `json:"state"`
	RuleID string       `json:"ruleID"`
	Hold   bool         `json:"hold"`
}

// newAssessObservation builds the observation record `watch` and `replay`
// both emit. A single construction site keeps the two JSONL streams
// field-for-field identical — calibration debugging diffs one against the
// other, so a field added to the output type must show up in both or not
// compile.
func newAssessObservation(ts time.Time, generation uint64, published terminal.Status, assessment assess.Assessment, debug status.DebugState, inMode bool) assessObservationOutput {
	return assessObservationOutput{
		Timestamp:      ts,
		Generation:     generation,
		Assessment:     assessStateOutput{State: assessment.State, RuleID: assessment.RuleID, Hold: assessment.Hold},
		Published:      published,
		Candidate:      debug.Candidate,
		CandidatePolls: debug.CandidatePolls,
		Churned:        debug.Churned,
		InMode:         inMode,
	}
}

// writeAssessObservation prints one observation either as a compact JSON
// line (jsonl, for scripting/analysis) or as a human-readable summary line
// (the default, for watching interactively).
func writeAssessObservation(w io.Writer, jsonl bool, out assessObservationOutput) error {
	if jsonl {
		data, err := json.Marshal(out)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(data))
		return err
	}

	_, err := fmt.Fprintf(w, "%s gen=%-4d state=%-9s rule=%-26s hold=%-5v -> %-8s candidate=%s(%d) churned=%-5v inMode=%v\n",
		out.Timestamp.Format(time.RFC3339), out.Generation,
		out.Assessment.State, orDash(out.Assessment.RuleID), out.Assessment.Hold,
		out.Published, orDash(string(out.Candidate)), out.CandidatePolls, out.Churned, out.InMode,
	)
	return err
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// assessFileCmd runs the assessment engine once over a captured pane file —
// the rule-authoring surface: see which text landed in which region and
// which rule fired, adjust a rule, rerun.
func (cmd *ExperimentalCmd) assessFileCmd() *cli.Command {
	var flagTool string

	return &cli.Command{
		Name:      "file",
		Usage:     "Assess a captured pane file once and print the result as JSON",
		UsageText: "hive x assess file <capture.txt> --tool <tool>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "tool",
				Aliases:     []string{"t"},
				Usage:       "Tool to assess as (claude, codex, ...); unrecognized tools use the generic rule set",
				Value:       "claude",
				Destination: &flagTool,
			},
		},
		Action: func(_ context.Context, c *cli.Command) error {
			if c.Args().Len() != 1 {
				return fmt.Errorf("usage: hive x assess file <capture.txt> --tool <tool>")
			}

			data, err := os.ReadFile(c.Args().First())
			if err != nil {
				return fmt.Errorf("reading capture file: %w", err)
			}
			content := string(data)

			assessment := assess.NewEngine().Assess(assess.Snapshot{Content: content, Tool: flagTool})
			dump := assess.DumpRegions(content)

			signals := assessment.Signals
			if signals == nil {
				signals = []assess.Signal{}
			}

			out := assessFileOutput{
				Tool:    flagTool,
				State:   assessment.State,
				Hold:    assessment.Hold,
				RuleID:  assessment.RuleID,
				Signals: signals,
				Regions: assessRegionsOutput{
					AboveBox:      dump.AboveBox,
					PromptBoxBody: dump.PromptBoxBody,
					BottomLines:   dump.BottomLines,
					AfterLastRule: dump.AfterLastRule,
				},
			}

			enc := json.NewEncoder(c.Root().Writer)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		},
	}
}
