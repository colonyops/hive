package commands

import (
	"context"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/pkg/iojson"
	"github.com/urfave/cli/v3"
)

type ConfigCmd struct {
	flags *Flags
	app   *app.App
}

// NewConfigCmd creates a new config command
func NewConfigCmd(flags *Flags, app *app.App) *ConfigCmd {
	return &ConfigCmd{flags: flags, app: app}
}

// Register adds the config command to the application
func (cmd *ConfigCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:      "config",
		Usage:     "Display resolved configuration",
		UsageText: "hive config",
		Description: `Dumps the fully resolved configuration as one JSON line.

This shows the effective configuration after loading the config file,
applying defaults, and merging keybindings.`,
		Metadata: alwaysJSON(),
		Action:   cmd.run,
	})

	return app
}

func (cmd *ConfigCmd) run(_ context.Context, c *cli.Command) error {
	return iojson.WriteLine(c.Root().Writer, cmd.app.Config)
}
