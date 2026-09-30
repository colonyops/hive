// Command release holds the release steps of the hive CLI.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/cmd/tools/release/internal/commands"
)

func main() {
	app := &cli.Command{
		Name:  "release",
		Usage: "release steps for the programs in this repository",
	}

	app = commands.NewCLICmd().Register(app)

	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
}
