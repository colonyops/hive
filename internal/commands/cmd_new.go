package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/hay-kot/hive/internal/hive"
	"github.com/hay-kot/hive/internal/printer"
	"github.com/hay-kot/hive/internal/styles"
	"github.com/hay-kot/hive/internal/templates"
	"github.com/urfave/cli/v3"
)

type NewCmd struct {
	flags *Flags

	// Command-specific flags
	name     string
	remote   string
	prompt   string
	template string
	setVals  []string
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
		UsageText: "hive new [options]",
		Description: `Creates a new isolated git environment for an AI agent session.

If a recyclable session exists for the same remote, it will be reused
(reset, checkout main, pull). Otherwise, a fresh clone is created.

After setup, any matching hooks are executed and the configured spawn
command launches a terminal with the AI tool.

When --name is omitted, an interactive form prompts for input.`,
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
			&cli.StringFlag{
				Name:        "template",
				Aliases:     []string{"t"},
				Usage:       "use a session template (run 'hive templates list' to see available)",
				Destination: &cmd.template,
			},
			&cli.StringSliceFlag{
				Name:        "set",
				Usage:       "set template field value (name=value), use commas for multi-select",
				Destination: &cmd.setVals,
			},
		},
		Action: cmd.run,
	})

	return app
}

func (cmd *NewCmd) run(ctx context.Context, c *cli.Command) error {
	p := printer.Ctx(ctx)

	// Template mode: use template to generate prompt and optionally name
	if cmd.template != "" {
		return cmd.runTemplate(ctx, p)
	}

	// Standard mode: show interactive form if name not provided via flag
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

	sess, err := cmd.flags.Service.CreateSession(ctx, opts)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	p.Success("Session created", sess.Path)

	return nil
}

func (cmd *NewCmd) runTemplate(ctx context.Context, p *printer.Printer) error {
	// Look up template from config
	tmpl, ok := cmd.flags.Config.Templates[cmd.template]
	if !ok {
		return fmt.Errorf("template %q not found (run 'hive templates list' to see available)", cmd.template)
	}

	var values map[string]any
	var err error

	// Use --set values if provided, otherwise run interactive form
	switch {
	case len(cmd.setVals) > 0:
		values, err = templates.ParseSetValues(cmd.setVals)
		if err != nil {
			return fmt.Errorf("parse --set values: %w", err)
		}

		// Validate required fields when using --set
		if err := templates.ValidateRequiredFields(tmpl, values); err != nil {
			return err
		}
	case len(tmpl.Fields) > 0:
		// Print banner header
		fmt.Println(styles.BannerStyle.Render(styles.Banner))
		fmt.Println()

		result, err := templates.RunForm(tmpl)
		if err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return nil
			}
			return fmt.Errorf("template form: %w", err)
		}
		values = result.Values
	default:
		values = make(map[string]any)
	}

	// Render prompt from template
	renderedPrompt, err := templates.RenderPrompt(tmpl, values)
	if err != nil {
		return fmt.Errorf("render prompt: %w", err)
	}

	// Render session name from template if provided, otherwise use flag or prompt for name
	sessionName := cmd.name
	if sessionName == "" && tmpl.Name != "" {
		sessionName, err = templates.RenderName(tmpl, values)
		if err != nil {
			return fmt.Errorf("render session name: %w", err)
		}
	}

	// If still no name, require user to provide one via form
	if sessionName == "" {
		if err := cmd.runNameForm(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return nil
			}
			return fmt.Errorf("name form: %w", err)
		}
		sessionName = cmd.name
	}

	opts := hive.CreateOptions{
		Name:   sessionName,
		Remote: cmd.remote,
		Prompt: renderedPrompt,
	}

	sess, err := cmd.flags.Service.CreateSession(ctx, opts)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	p.Success("Session created", sess.Path)

	return nil
}

func (cmd *NewCmd) runNameForm() error {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Session name").
				Description("Used in the directory path").
				Validate(validateName).
				Value(&cmd.name),
		),
	).WithTheme(styles.FormTheme()).Run()
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
