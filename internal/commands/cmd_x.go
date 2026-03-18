package commands

import (
	"github.com/colonyops/hive/internal/hive"
	"github.com/urfave/cli/v3"
)

// ExperimentalCmd implements the hive x command group.
type ExperimentalCmd struct {
	flags *Flags
	app   *hive.App
}

// NewExperimentalCmd creates a new experimental command group.
func NewExperimentalCmd(flags *Flags, app *hive.App) *ExperimentalCmd {
	return &ExperimentalCmd{flags: flags, app: app}
}

// Register adds the x command group to the application.
func (cmd *ExperimentalCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:  "x",
		Usage: "Experimental commands",
		Commands: []*cli.Command{
			cmd.pickCmd(),
		},
	})
	return app
}
