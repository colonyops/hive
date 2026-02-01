package commands

import (
	"context"
	"strings"

	initcmd "github.com/hay-kot/hive/internal/commands/init"
	"github.com/urfave/cli/v3"
)

type InitCmd struct {
	flags    *Flags
	yes      bool
	force    bool
	repoDirs string
	noScript bool
}

func NewInitCmd(flags *Flags) *InitCmd {
	return &InitCmd{flags: flags}
}

func (cmd *InitCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:      "init",
		Usage:     "Initialize hive configuration with an interactive wizard",
		UsageText: "hive init [options]",
		Description: `Sets up hive for first-time use with an interactive wizard.

The wizard will:
  - Generate ~/.config/hive/config.yaml with sensible defaults
  - Install the hive.sh helper script to ~/.local/bin/
  - Optionally add a shell alias (hv) to your shell config
  - Optionally configure tmux keybindings

Use --yes to accept all defaults without prompts.
Use --force to overwrite existing configuration.`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "yes",
				Aliases:     []string{"y"},
				Usage:       "accept defaults without prompting",
				Destination: &cmd.yes,
			},
			&cli.BoolFlag{
				Name:        "force",
				Aliases:     []string{"f"},
				Usage:       "overwrite existing configuration",
				Destination: &cmd.force,
			},
			&cli.StringFlag{
				Name:        "repo-dirs",
				Usage:       "comma-separated list of repository directories",
				Destination: &cmd.repoDirs,
			},
			&cli.BoolFlag{
				Name:        "no-script",
				Usage:       "skip helper script installation",
				Destination: &cmd.noScript,
			},
		},
		Action: cmd.run,
	})
	return app
}

// RepoDirsList returns the parsed list of repo directories, or nil if not set.
func (cmd *InitCmd) RepoDirsList() []string {
	if cmd.repoDirs == "" {
		return nil
	}
	dirs := strings.Split(cmd.repoDirs, ",")
	for i, d := range dirs {
		dirs[i] = strings.TrimSpace(d)
	}
	return dirs
}

func (cmd *InitCmd) run(ctx context.Context, c *cli.Command) error {
	wizard := initcmd.NewWizard(initcmd.WizardOptions{
		ConfigPath: cmd.flags.ConfigPath,
		Yes:        cmd.yes,
		Force:      cmd.force,
		RepoDirs:   cmd.RepoDirsList(),
		NoScript:   cmd.noScript,
	})
	return wizard.Run(ctx)
}
