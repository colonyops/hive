// Command docgen generates CLI reference documentation from the hive command
// definitions. Output is written to docs/cli-reference.md.
package main

import (
	"fmt"
	"os"

	docs "github.com/urfave/cli-docs/v3"
	"github.com/urfave/cli/v3"

	"github.com/hay-kot/hive/internal/commands"
	"github.com/hay-kot/hive/internal/hive"
)

func main() {
	flags := &commands.Flags{}
	app := &hive.App{}

	root := &cli.Command{
		Name:      "hive",
		Usage:     "Manage multiple AI agent sessions",
		UsageText: "hive [global options] command [command options]",
		Description: `Hive creates isolated git environments for running multiple AI agents in parallel.

Instead of managing worktrees manually, hive handles cloning, recycling, and
spawning terminal sessions with your preferred AI tool.

Run 'hive' with no arguments to open the interactive session manager.
Run 'hive new' to create a new session from the current repository.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "log-level",
				Usage:   "log level (debug, info, warn, error, fatal, panic)",
				Sources: cli.EnvVars("HIVE_LOG_LEVEL"),
				Value:   "info",
			},
			&cli.StringFlag{
				Name:    "log-file",
				Usage:   "path to log file (defaults to <data-dir>/hive.log)",
				Sources: cli.EnvVars("HIVE_LOG_FILE"),
			},
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "path to config file",
				Sources: cli.EnvVars("HIVE_CONFIG"),
				Value:   commands.DefaultConfigPath(),
			},
			&cli.StringFlag{
				Name:    "data-dir",
				Usage:   "path to data directory",
				Sources: cli.EnvVars("HIVE_DATA_DIR"),
				Value:   commands.DefaultDataDir(),
			},
		},
	}

	tuiCmd := commands.NewTuiCmd(flags, app)
	root.Flags = append(root.Flags, tuiCmd.Flags()...)

	root = commands.NewNewCmd(flags, app).Register(root)
	root = commands.NewLsCmd(flags, app).Register(root)
	root = commands.NewPruneCmd(flags, app).Register(root)
	root = commands.NewDoctorCmd(flags, app).Register(root)
	root = commands.NewBatchCmd(flags, app).Register(root)
	root = commands.NewCtxCmd(flags, app).Register(root)
	root = commands.NewMsgCmd(flags, app).Register(root)
	root = commands.NewDocCmd(flags, app).Register(root)
	root = commands.NewSessionCmd(flags, app).Register(root)
	root = commands.NewReviewCmd(flags, app).Register(root)

	md, err := docs.ToMarkdown(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error generating docs: %v\n", err)
		os.Exit(1)
	}

	outPath := "docs/cli-reference.md"
	if len(os.Args) > 1 {
		outPath = os.Args[1]
	}

	if err := os.WriteFile(outPath, []byte(md), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", outPath, err)
		os.Exit(1)
	}

	fmt.Printf("Generated %s\n", outPath)
}
