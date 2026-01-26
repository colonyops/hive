package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/hay-kot/hive/internal/core/history"
	"github.com/hay-kot/hive/internal/hive"
	"github.com/hay-kot/hive/internal/printer"
	"github.com/hay-kot/hive/internal/styles"
	"github.com/urfave/cli/v3"
)

type NewCmd struct {
	flags *Flags

	// Command-specific flags
	name   string
	remote string
	prompt string
	replay bool
}

// NewNewCmd creates a new new command
func NewNewCmd(flags *Flags) *NewCmd {
	return &NewCmd{flags: flags}
}

// Register adds the new command to the application
func (cmd *NewCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:      "new",
		Usage:     "Create a new agent session",
		UsageText: "hive new [options] [replay-id]",
		Description: `Creates a new isolated git environment for an AI agent session.

If a recyclable session exists for the same remote, it will be reused
(reset, checkout main, pull). Otherwise, a fresh clone is created.

After setup, any matching hooks are executed and the configured spawn
command launches a terminal with the AI tool.

When --name is omitted, an interactive form prompts for input.

Use --replay to re-run a previous command:
  hive new --replay          # Replay the last failed 'new' command
  hive new --replay <id>     # Replay a specific command by ID`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "name",
				Aliases:     []string{"n"},
				Usage:       "session name used in the directory path",
				Destination: &cmd.name,
			},
			&cli.StringFlag{
				Name:        "remote",
				Aliases:     []string{"r"},
				Usage:       "git remote URL (defaults to current directory's origin)",
				Destination: &cmd.remote,
			},
			&cli.StringFlag{
				Name:        "prompt",
				Aliases:     []string{"p"},
				Usage:       "AI prompt passed to the spawn command template",
				Destination: &cmd.prompt,
			},
			&cli.BoolFlag{
				Name:        "replay",
				Aliases:     []string{"R"},
				Usage:       "replay a previous command (last failed, or specify ID as argument)",
				Destination: &cmd.replay,
			},
		},
		Action: cmd.run,
	})

	return app
}

func (cmd *NewCmd) run(ctx context.Context, c *cli.Command) error {
	p := printer.Ctx(ctx)

	// Handle replay mode
	if cmd.replay {
		return cmd.runReplay(ctx, c, p)
	}

	// Show interactive form if name not provided via flag
	if cmd.name == "" {
		if err := cmd.runForm(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return nil
			}
			return fmt.Errorf("form: %w", err)
		}
	}

	opts := hive.CreateOptions{
		Name:   cmd.name,
		Remote: cmd.remote,
		Prompt: cmd.prompt,
	}

	// Save parsed options for history recording
	cmd.flags.LastNewOptions = &history.NewOptions{
		Name:   cmd.name,
		Remote: cmd.remote,
		Prompt: cmd.prompt,
	}

	sess, err := cmd.flags.Service.CreateSession(ctx, opts)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	p.Success("Session created", sess.Path)

	return nil
}

func (cmd *NewCmd) runReplay(ctx context.Context, c *cli.Command, p *printer.Printer) error {
	var entry history.Entry
	var err error

	// Check if an ID was provided as a positional argument
	replayID := c.Args().First()

	if replayID == "" {
		// Replay last command
		entry, err = cmd.flags.HistoryStore.Last(ctx)
		if errors.Is(err, history.ErrNotFound) {
			p.Infof("No commands in history")
			return nil
		}
		if err != nil {
			return fmt.Errorf("get last command: %w", err)
		}
	} else {
		// Replay specific command by ID
		entry, err = cmd.flags.HistoryStore.Get(ctx, replayID)
		if errors.Is(err, history.ErrNotFound) {
			return fmt.Errorf("command %q not found in history", replayID)
		}
		if err != nil {
			return fmt.Errorf("get command: %w", err)
		}
	}

	cmdStr := entry.Command
	if len(entry.Args) > 0 {
		cmdStr += " " + strings.Join(entry.Args, " ")
	}
	p.Infof("Replaying: hive %s", cmdStr)

	// Use stored options if available, otherwise parse from args (backward compat)
	var opts hive.CreateOptions
	if entry.Options != nil {
		opts = hive.CreateOptions{
			Name:   entry.Options.Name,
			Remote: entry.Options.Remote,
			Prompt: entry.Options.Prompt,
		}
	} else {
		opts, err = cmd.parseArgsToOptions(entry.Args)
		if err != nil {
			return fmt.Errorf("parse replay args: %w", err)
		}
	}

	sess, err := cmd.flags.Service.CreateSession(ctx, opts)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	p.Success("Session created", sess.Path)
	return nil
}

// parseArgsToOptions parses CLI args back into CreateOptions.
func (cmd *NewCmd) parseArgsToOptions(args []string) (hive.CreateOptions, error) {
	var opts hive.CreateOptions

	for i := 0; i < len(args); i++ {
		arg := args[i]

		// Handle --flag=value format
		if strings.HasPrefix(arg, "--name=") {
			opts.Name = strings.TrimPrefix(arg, "--name=")
			continue
		}
		if strings.HasPrefix(arg, "--remote=") {
			opts.Remote = strings.TrimPrefix(arg, "--remote=")
			continue
		}
		if strings.HasPrefix(arg, "--prompt=") {
			opts.Prompt = strings.TrimPrefix(arg, "--prompt=")
			continue
		}

		// Handle --flag value format
		switch arg {
		case "-n", "--name":
			if i+1 < len(args) {
				opts.Name = args[i+1]
				i++
			}
		case "-r", "--remote":
			if i+1 < len(args) {
				opts.Remote = args[i+1]
				i++
			}
		case "-p", "--prompt":
			if i+1 < len(args) {
				opts.Prompt = args[i+1]
				i++
			}
		}
	}

	if opts.Name == "" {
		return opts, fmt.Errorf("no session name found in history entry")
	}

	return opts, nil
}

func (cmd *NewCmd) runForm() error {
	// Print banner header
	fmt.Println(styles.BannerStyle.Render(styles.Banner))
	fmt.Println()

	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Session name").
				Description("Used in the directory path").
				Validate(validateName).
				Value(&cmd.name),
			huh.NewText().
				Title("Prompt").
				Description("AI prompt to pass to spawn command").
				Value(&cmd.prompt),
		),
	).WithTheme(styles.FormTheme()).Run()
}

func validateName(s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}
