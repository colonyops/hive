package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/internal/core/terminal/assess"
)

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

// assessCmd registers the "hive x assess" command group. Only "file" exists
// in this phase; "watch"/"replay" land with the status.Tracker (Phase 3).
func (cmd *ExperimentalCmd) assessCmd() *cli.Command {
	return &cli.Command{
		Name:  "assess",
		Usage: "Status assessment engine debug tooling (rule-authoring aid)",
		Commands: []*cli.Command{
			cmd.assessFileCmd(),
		},
	}
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
