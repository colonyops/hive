package commands

import (
	"context"
	"os"

	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/pkg/iojson"
	"github.com/urfave/cli/v3"
)

type ConfigCmd struct {
	flags *Flags
	app   *hive.App
}

// NewConfigCmd creates a new config command
func NewConfigCmd(flags *Flags, app *hive.App) *ConfigCmd {
	return &ConfigCmd{flags: flags, app: app}
}

// Register adds the config command to the application
func (cmd *ConfigCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:      "config",
		Usage:     "Display resolved configuration",
		UsageText: "hive config",
		Description: `Dumps the fully resolved configuration as pretty-printed JSON.

This shows the effective configuration after loading the config file,
applying defaults, and merging keybindings.`,
		Action: cmd.run,
	})

	return app
}

func (cmd *ConfigCmd) run(_ context.Context, c *cli.Command) error {
	return iojson.WriteWith(c.Root().Writer, os.Stderr, cmd.app.Config)
}
