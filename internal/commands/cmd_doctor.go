package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/hay-kot/hive/internal/core/doctor"
	"github.com/hay-kot/hive/internal/core/styles"
	"github.com/hay-kot/hive/internal/hive"
	"github.com/hay-kot/hive/pkg/iojson"
	"github.com/urfave/cli/v3"
)

type DoctorCmd struct {
	flags   *Flags
	app     *hive.App
	format  string
	autofix bool
}

func NewDoctorCmd(flags *Flags, app *hive.App) *DoctorCmd {
	return &DoctorCmd{flags: flags, app: app}
}

func (cmd *DoctorCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:        "doctor",
		Usage:       "Run health checks on your hive setup",
		UsageText:   "hive doctor [options]",
		Description: "Runs diagnostic checks on configuration, environment, and dependencies.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "format",
				Usage:       "output format (text, json)",
				Value:       "text",
				Destination: &cmd.format,
			},
			&cli.BoolFlag{
				Name:        "autofix",
				Usage:       "automatically fix issues (e.g., delete orphaned worktrees)",
				Destination: &cmd.autofix,
			},
		},
		Action: cmd.run,
	})
	return app
}

func (cmd *DoctorCmd) run(ctx context.Context, c *cli.Command) error {
	results := cmd.app.Doctor.RunChecks(ctx, cmd.flags.ConfigPath, cmd.autofix)

	if cmd.format == "json" {
		return cmd.outputJSON(c, results)
	}

	return cmd.outputText(ctx, results)
}

func (cmd *DoctorCmd) outputJSON(c *cli.Command, results []doctor.Result) error {
	passed, warned, failed := doctor.Summary(results)

	out := struct {
		Healthy bool            `json:"healthy"`
		Summary summaryJSON     `json:"summary"`
		Checks  []doctor.Result `json:"checks"`
	}{
		Healthy: failed == 0,
		Summary: summaryJSON{Passed: passed, Warned: warned, Failed: failed},
		Checks:  results,
	}

	return iojson.WriteWith(c.Root().Writer, os.Stderr, out)
}

type summaryJSON struct {
	Passed int `json:"passed"`
	Warned int `json:"warned"`
	Failed int `json:"failed"`
}

func (cmd *DoctorCmd) outputText(_ context.Context, results []doctor.Result) error {
	w := os.Stderr
	divider := styles.TextMutedStyle.Render(strings.Repeat("─", 40))

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, styles.TextPrimaryBoldStyle.Render("Hive Doctor"))
	_, _ = fmt.Fprintln(w, divider)
	_, _ = fmt.Fprintln(w)

	for _, result := range results {
		_, _ = fmt.Fprintln(w, styles.TextForegroundBoldStyle.Render(result.Name))

		for _, item := range result.Items {
			var detail string
			if item.Detail != "" {
				detail = " " + styles.TextMutedStyle.Render(item.Detail)
			}

			var icon string
			switch item.Status {
			case doctor.StatusPass:
				icon = styles.TextSuccessStyle.Render("✔")
			case doctor.StatusWarn:
				icon = styles.TextWarningStyle.Render("●")
			case doctor.StatusFail:
				icon = styles.TextErrorStyle.Render("✘")
			}

			_, _ = fmt.Fprintf(w, "  %s %s%s\n", icon, item.Label, detail)
		}

		_, _ = fmt.Fprintln(w)
	}

	passed, warned, failed := doctor.Summary(results)
	summary := fmt.Sprintf("%s  %s  %s",
		styles.TextSuccessStyle.Render(fmt.Sprintf("%d passed", passed)),
		styles.TextWarningStyle.Render(fmt.Sprintf("%d warnings", warned)),
		styles.TextErrorStyle.Render(fmt.Sprintf("%d failed", failed)),
	)
	_, _ = fmt.Fprintln(w, summary)

	if !cmd.autofix {
		fixable := doctor.CountFixable(results)
		if fixable > 0 {
			_, _ = fmt.Fprintln(w)
			hint := styles.TextMutedStyle.Render(fmt.Sprintf("Run 'hive doctor --autofix' to fix %d issue(s)", fixable))
			_, _ = fmt.Fprintln(w, hint)
		}
	}

	if failed > 0 {
		return cli.Exit("", 1)
	}

	return nil
}
