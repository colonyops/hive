package commands

import (
	"context"
	"time"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/internal/domain/usageanalytics"
	"github.com/urfave/cli/v3"
)

// InstrumentCommands captures registered paths, never the parsed command line.
func InstrumentCommands(root *cli.Command, facade *app.App) {
	var visit func(*cli.Command, string)
	visit = func(parent *cli.Command, prefix string) {
		for _, command := range parent.Commands {
			path := prefix + command.Name
			if id, ok := usageanalytics.LookupCommand(path); ok && command.Action != nil {
				action := command.Action
				command.Action = func(ctx context.Context, c *cli.Command) error {
					started := time.Now()
					err := action(ctx, c)
					if facade.Analytics != nil {
						facade.Analytics.Record(ctx, usageanalytics.CommandCompleted(id, err == nil, time.Since(started)))
					}
					return err
				}
			}
			visit(command, path+".")
		}
	}
	visit(root, "")
}
