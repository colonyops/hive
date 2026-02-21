package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/tui/install"
)

type InstallCmd struct {
	flags *Flags
	app   *hive.App
}

func NewInstallCmd(flags *Flags, app *hive.App) *InstallCmd {
	return &InstallCmd{flags: flags, app: app}
}

func (cmd *InstallCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:        "install",
		Usage:       "Interactive setup wizard for hive",
		UsageText:   "hive install",
		Description: "Guides you through configuring workspaces, config files, and shell aliases.",
		Action:      cmd.run,
	})
	return app
}

func (cmd *InstallCmd) run(ctx context.Context, c *cli.Command) error {
	m := install.New()

	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("run install wizard: %w", err)
	}

	result := finalModel.(install.Model).Result()

	if result.Cancelled {
		fmt.Println("Setup cancelled.")
		return nil
	}

	// TODO: Write/update config file with workspaces

	// Add shell aliases
	var addedShells []install.ShellConfig
	for _, shell := range result.SelectedShells {
		if err := addShellAlias(shell); err != nil {
			fmt.Printf("Warning: failed to add alias to %s: %v\n", shell.Path, err)
		} else {
			fmt.Printf("Added 'hv' alias to %s\n", shell.Path)
			addedShells = append(addedShells, shell)
		}
	}

	fmt.Println()
	fmt.Println(styles.TextSuccessStyle.Render("Setup complete!"))

	// Check if common agent commands are available
	checkAgentCommands()

	// Show source command for modified shells
	if len(addedShells) > 0 {
		fmt.Println()
		// Just show the first one - they likely only need one
		sourceCmd := fmt.Sprintf("source %s", addedShells[0].Path)
		fmt.Println("Run this to activate the alias:")
		fmt.Println()
		fmt.Println(styles.TextPrimaryBoldStyle.Render("  " + sourceCmd))
		fmt.Println()
		fmt.Println("Then use " + styles.TextPrimaryBoldStyle.Render("hv") + " to launch Hive.")
	}

	return nil
}

// checkAgentCommands verifies that common agent commands are available in PATH.
func checkAgentCommands() {
	agents := []string{"claude", "codex"}
	var missing []string

	for _, agent := range agents {
		if _, err := exec.LookPath(agent); err != nil {
			missing = append(missing, agent)
		}
	}

	if len(missing) > 0 {
		fmt.Println()
		fmt.Println(styles.TextWarningStyle.Render("Warning: ") + "The following agents were not found in PATH:")
		for _, agent := range missing {
			fmt.Println(styles.TextMutedStyle.Render("  - " + agent))
		}
		fmt.Println(styles.TextMutedStyle.Render("Install them or ensure they're in your PATH before creating sessions."))
	}
}

// addShellAlias appends the hive alias to a shell config file.
func addShellAlias(shell install.ShellConfig) error {
	// Check if alias already exists
	content, err := os.ReadFile(shell.Path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Check for existing alias
	if strings.Contains(string(content), "alias hv=") || strings.Contains(string(content), "alias hv ") {
		// Alias already exists
		return nil
	}

	// Determine alias syntax based on shell type
	// The alias runs hive inside a tmux session named "hive"
	// -A attaches to existing session if it exists, -s names the session
	var aliasLine string
	if shell.Name == "fish" {
		aliasLine = "\n# Hive alias\nalias hv 'tmux new-session -As hive hive'\n"
	} else {
		// bash, zsh, bash_profile
		aliasLine = "\n# Hive alias\nalias hv='tmux new-session -As hive hive'\n"
	}

	// Append to file
	f, err := os.OpenFile(shell.Path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(aliasLine); err != nil {
		return fmt.Errorf("write alias: %w", err)
	}

	return nil
}
