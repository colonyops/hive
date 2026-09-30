// Command release holds the release steps of the hive CLI.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"
)

func main() {
	if err := newReleaseCommand().Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
}

func newReleaseCommand() *cli.Command {
	return &cli.Command{
		Name:     "release",
		Usage:    "release steps for the programs in this repository",
		Commands: []*cli.Command{newCLICommand()},
	}
}
