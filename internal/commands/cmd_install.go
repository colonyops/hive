package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"

	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/skillsinstall"
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
		Commands: []*cli.Command{
			{
				Name:        "skills",
				Usage:       "Install bundled AI skills and commands",
				UsageText:   "hive install skills [--claude] [--codex] [--force]",
				Description: "Extracts bundled skills and commands to ~/.config/hive/ and symlinks them into agent config directories.",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "claude", Value: true, Usage: "install for Claude Code (~/.claude)"},
					&cli.BoolFlag{Name: "codex", Value: true, Usage: "install for Codex (~/.codex)"},
					&cli.BoolFlag{Name: "force", Usage: "overwrite existing non-symlink files"},
				},
				Action: cmd.runSkills,
			},
		},
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

	// Write/update config file with workspaces
	if err := writeConfig(result.ConfigPath, result.Workspaces); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	fmt.Printf("Config written to %s\n", result.ConfigPath)

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

	// Install skills if the user selected any agents.
	if len(result.SkillAgents) > 0 {
		if err := cmd.installSkillsFor(result.SkillAgents, false); err != nil {
			fmt.Printf("Warning: failed to install skills: %v\n", err)
		}
	}

	fmt.Println()
	fmt.Println(styles.TextSuccessStyle.Render("Setup complete!"))

	checkAgentCommands()

	if len(addedShells) > 0 {
		fmt.Println()
		sourceCmd := fmt.Sprintf("source %s", addedShells[0].Path)
		fmt.Println("Run this to activate the alias:")
		fmt.Println()
		fmt.Println(styles.TextPrimaryBoldStyle.Render("  " + sourceCmd))
		fmt.Println()
		fmt.Println("Then use " + styles.TextPrimaryBoldStyle.Render("hv") + " to launch Hive.")
	}

	return nil
}

func (cmd *InstallCmd) runSkills(ctx context.Context, c *cli.Command) error {
	var agents []string
	if c.Bool("claude") {
		agents = append(agents, string(skillsinstall.AgentClaude))
	}
	if c.Bool("codex") {
		agents = append(agents, string(skillsinstall.AgentCodex))
	}

	if len(agents) == 0 {
		fmt.Println("No agents selected. Use --claude or --codex.")
		return nil
	}

	return cmd.installSkillsFor(agents, c.Bool("force"))
}

// installSkillsFor runs the skills installer for the given agent names.
func (cmd *InstallCmd) installSkillsFor(agents []string, force bool) error {
	var skillAgents []skillsinstall.Agent
	for _, a := range agents {
		agent, ok := skillsinstall.ParseAgent(a)
		if !ok {
			fmt.Printf("Warning: unknown agent %q — skipping\n", a)
			continue
		}
		skillAgents = append(skillAgents, agent)
	}

	if len(skillAgents) == 0 {
		return nil
	}

	installer := &skillsinstall.Installer{
		ConfigDir: defaultConfigDir(),
		Agents:    skillAgents,
		Force:     force,
	}

	if err := installer.Install(); err != nil {
		return err
	}

	configDir := defaultConfigDir()
	fmt.Printf("Skills extracted to %s/skills/\n", configDir)
	fmt.Printf("Commands extracted to %s/commands/\n", configDir)
	for _, agent := range skillAgents {
		switch agent {
		case skillsinstall.AgentClaude:
			fmt.Println("Linked ~/.claude/skills and ~/.claude/commands")
		case skillsinstall.AgentCodex:
			fmt.Println("Linked ~/.codex/skills/claude and ~/.codex/prompts")
		}
	}

	return nil
}

// writeConfig writes workspaces to the config file, preserving any existing keys.
func writeConfig(configPath string, workspaces []string) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	// Load existing config as a raw map to preserve unknown keys
	existing := map[string]any{}
	if data, err := os.ReadFile(configPath); err == nil {
		_ = yaml.Unmarshal(data, &existing)
	}

	existing["workspaces"] = workspaces

	data, err := yaml.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	return os.WriteFile(configPath, data, 0o644)
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
	content, err := os.ReadFile(shell.Path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	if strings.Contains(string(content), "alias hv=") || strings.Contains(string(content), "alias hv ") {
		return nil
	}

	var aliasLine string
	if shell.Name == "fish" {
		aliasLine = "\n# Hive alias\nalias hv 'tmux new-session -As hive hive'\n"
	} else {
		aliasLine = "\n# Hive alias\nalias hv='tmux new-session -As hive hive'\n"
	}

	f, err := os.OpenFile(shell.Path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	if _, err = f.WriteString(aliasLine); err != nil {
		return fmt.Errorf("write alias: %w", err)
	}

	return nil
}
