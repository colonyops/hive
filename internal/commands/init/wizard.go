package initcmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/hay-kot/hive/internal/commands/doctor"
	"github.com/hay-kot/hive/internal/printer"
)

// WizardOptions configures the wizard behavior.
type WizardOptions struct {
	ConfigPath string
	Yes        bool     // skip prompts, use defaults
	Force      bool     // overwrite existing config
	RepoDirs   []string // pre-specified repo dirs (nil = prompt)
	NoScript   bool     // skip hive.sh installation
}

// Wizard orchestrates the init process.
type Wizard struct {
	opts WizardOptions
}

// NewWizard creates a new init wizard.
func NewWizard(opts WizardOptions) *Wizard {
	return &Wizard{opts: opts}
}

// Run executes the wizard.
func (w *Wizard) Run(ctx context.Context) error {
	p := printer.Ctx(ctx)

	// Check for existing config
	if ConfigExists(w.opts.ConfigPath) && !w.opts.Force {
		if w.opts.Yes {
			return fmt.Errorf("config exists at %s; use --force to overwrite", w.opts.ConfigPath)
		}

		var overwrite bool
		err := huh.NewConfirm().
			Title("Config file already exists").
			Description(w.opts.ConfigPath + "\nOverwrite? (a backup will be created)").
			Value(&overwrite).
			Run()
		if err != nil {
			return err
		}
		if !overwrite {
			p.Infof("Init cancelled")
			return nil
		}
	}

	// Detect tmux
	hasTmux := TmuxAvailable()
	if !hasTmux {
		p.Warnf("tmux not found - terminal integration will be disabled")
	}

	// Collect configuration
	repoDirs := w.opts.RepoDirs
	installScript := !w.opts.NoScript && hasTmux
	installAlias := hasTmux
	installTmuxConfig := hasTmux

	if !w.opts.Yes {
		var err error
		repoDirs, installScript, installAlias, installTmuxConfig, err = w.promptUser(hasTmux, repoDirs)
		if err != nil {
			return err
		}
	}

	// Use defaults if not specified
	if len(repoDirs) == 0 {
		repoDirs = DefaultRepoDirs()
	}

	// Expand ~ in paths
	for i, dir := range repoDirs {
		repoDirs[i] = expandHome(dir)
	}

	// Backup existing config if needed
	if ConfigExists(w.opts.ConfigPath) {
		backupPath, err := BackupConfig(w.opts.ConfigPath)
		if err != nil {
			return fmt.Errorf("backup config: %w", err)
		}
		if backupPath != "" {
			p.Successf("Backed up config to: %s", backupPath)
		}
	}

	// Install hive.sh if requested
	var scriptPath string
	if installScript {
		var err error
		scriptPath, err = InstallHiveScript()
		if err != nil {
			p.Warnf("Failed to install hive.sh: %v", err)
		} else {
			p.Successf("Installed helper script: %s", scriptPath)
		}
	}

	// Generate and write config
	cfg := GenerateConfig(ConfigOptions{
		RepoDirs:    repoDirs,
		InstallTmux: hasTmux && installScript,
		ScriptPath:  scriptPath,
	})

	if err := WriteConfig(cfg, w.opts.ConfigPath); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	p.Successf("Created config: %s", w.opts.ConfigPath)

	// Setup shell alias if requested
	if installAlias {
		shell, err := DetectShell()
		if err != nil {
			p.Warnf("Could not detect shell: %v", err)
		} else if err := SetupShellAlias(shell); err != nil {
			p.Warnf("Failed to setup shell alias: %v", err)
		} else {
			p.Successf("Added alias to %s", shell.RCFile)
		}
	}

	// Setup tmux config if requested
	if installTmuxConfig {
		if err := SetupTmuxConfig(); err != nil {
			p.Warnf("Failed to setup tmux config: %v", err)
		} else {
			p.Successf("Added keybindings to %s", TmuxConfigPath())
		}
	}

	// Run validation checks
	p.Printf("")
	check := NewInitCheck(w.opts.ConfigPath)
	result := check.Run(ctx)

	p.Section(result.Name)
	for _, item := range result.Items {
		switch item.Status {
		case doctor.StatusPass:
			p.CheckItem(item.Label, item.Detail)
		case doctor.StatusWarn:
			p.WarnItem(item.Label, item.Detail)
		case doctor.StatusFail:
			p.FailItem(item.Label, item.Detail)
		}
	}

	// Print next steps
	w.printNextSteps(p, installAlias, installTmuxConfig)

	return nil
}

func (w *Wizard) promptUser(hasTmux bool, presetRepoDirs []string) (repoDirs []string, installScript, installAlias, installTmuxConfig bool, err error) {
	// Default values
	repoDirsStr := strings.Join(DefaultRepoDirs(), ", ")
	if len(presetRepoDirs) > 0 {
		repoDirsStr = strings.Join(presetRepoDirs, ", ")
	}
	installScript = hasTmux
	installAlias = hasTmux
	installTmuxConfig = hasTmux

	fields := []huh.Field{
		huh.NewInput().
			Title("Repository directories").
			Description("Comma-separated list of directories containing git repos").
			Value(&repoDirsStr),
	}

	if hasTmux {
		fields = append(fields,
			huh.NewConfirm().
				Title("Install hive.sh helper script?").
				Description("Enables tmux integration for spawning sessions").
				Value(&installScript),
			huh.NewConfirm().
				Title("Add shell alias (hv)?").
				Description("Quick command to launch hive in tmux").
				Value(&installAlias),
			huh.NewConfirm().
				Title("Add tmux keybindings?").
				Description("Adds 'prefix h' to open hive popup").
				Value(&installTmuxConfig),
		)
	}

	form := huh.NewForm(huh.NewGroup(fields...))
	if err = form.Run(); err != nil {
		return
	}

	// Parse repo dirs
	for _, dir := range strings.Split(repoDirsStr, ",") {
		dir = strings.TrimSpace(dir)
		if dir != "" {
			repoDirs = append(repoDirs, dir)
		}
	}

	return repoDirs, installScript, installAlias, installTmuxConfig, nil
}

func (w *Wizard) printNextSteps(p *printer.Printer, installedAlias, installedTmux bool) {
	p.Printf("")
	p.Section("Next Steps")

	step := 1
	if installedAlias {
		shell, err := DetectShell()
		if err == nil {
			p.Printf("  %d. Run 'source %s' or restart your shell", step, shell.RCFile)
			step++
		}
	}

	if installedTmux {
		p.Printf("  %d. Run 'tmux source %s' to reload tmux", step, TmuxConfigPath())
		step++
	}

	p.Printf("  %d. Run 'hive' to start using hive", step)
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
