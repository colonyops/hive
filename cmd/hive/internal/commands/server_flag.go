package commands

import (
	"context"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/cmd/hive/internal/app"
)

// serverFlag picks the server under servers: that a session command acts on.
// A bare session id then names a session there; "<server>:<id>" still wins.
func serverFlag(dest *string) cli.Flag {
	return &cli.StringFlag{
		Name:        "server",
		Usage:       "act on the named server under servers: (local is this machine)",
		Sources:     cli.EnvVars("HIVE_SERVER"),
		Destination: dest,
	}
}

// targetServer is the Before hook of a command with serverFlag.
func targetServer(a *app.App, name *string) cli.BeforeFunc {
	return func(ctx context.Context, _ *cli.Command) (context.Context, error) {
		if *name == "" {
			return ctx, nil
		}
		return ctx, a.TargetServer(*name)
	}
}
