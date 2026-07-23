package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"charm.land/bubbles/v2/key"
	"charm.land/huh/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/hive"
	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"
)

type agentTemplateData struct {
	Name  string
	Flags []string
}

type configTemplateData struct {
	Version   string
	Workspace string
	Default   string
	Agents    []agentTemplateData
}

type resultStatus int

const (
	statusDone resultStatus = iota
	statusSkipped
	statusFailed
)

type stepResult struct {
	name    string
	status  resultStatus
	detail  string
	fixHint string
}

var knownAgents = []string{"claude", "opencode", "codex", "pi", "amp", "copilot", "cursor"}

var agentFlagMap = map[string][]string{
	"claude":   {"--dangerously-skip-permissions"},
	"opencode": {"--agent", "free-permissions-runner"},
	"codex":    {"--full-auto"},
}

// detectInstalledAgents returns the subset of known agent names found on PATH,
// preserving input order. Uses exec.LookPath.
func detectInstalledAgents(known []string) []string {
	result := []string{}
	for _, name := range known {
		if _, err := exec.LookPath(name); err == nil {
			result = append(result, name)
		}
	}
	return result
}

// detectShell inspects $SHELL and returns a short name and the rc file path.
// Returns ("unknown", "") when $SHELL is unset, empty, or unrecognised.
// Returns (shellName, "") when the shell is recognised but the home directory
// is unavailable; callers should treat an empty rcFile as a failure.
// The bash case performs a file existence check on ~/.bash_profile.
func detectShell() (shellName, rcFile string) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return "unknown", ""
	}

	switch {
	case strings.HasSuffix(shell, "zsh"):
		shellName = "zsh"
	case strings.HasSuffix(shell, "bash"):
		shellName = "bash"
	case strings.HasSuffix(shell, "fish"):
		shellName = "fish"
	default:
		return "unknown", ""
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return shellName, ""
	}

	switch shellName {
	case "zsh":
		return shellName, filepath.Join(home, ".zshrc")
	case "bash":
		bashProfile := filepath.Join(home, ".bash_profile")
		if _, err := os.Stat(bashProfile); err == nil {
			return shellName, bashProfile
		}
		return shellName, filepath.Join(home, ".bashrc")
	case "fish":
		return shellName, filepath.Join(home, ".config", "fish", "config.fish")
	}
	return "unknown", ""
}

// aliasAlreadyPresent reports whether rcFile contains "alias hv" on any line.
// Returns (false, nil) if the file does not exist.
func aliasAlreadyPresent(rcFile, aliasName string) (bool, error) {
	data, err := os.ReadFile(rcFile)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	lines := strings.Split(string(data), "\n")
	needle := "alias " + aliasName
	for _, line := range lines {
		if strings.Contains(line, needle) {
			return true, nil
		}
	}
	return false, nil
}

// appendAlias appends the hv alias line to rcFile.
// Fish uses `alias hv '...'`; all other shells use `alias hv='...'`.
func appendAlias(rcFile, shellName string) error {
	var line string
	if shellName == "fish" {
		line = "\nalias hv 'tmux new-session -As hive hive'\n"
	} else {
		line = "\nalias hv='tmux new-session -As hive hive'\n"
	}

	f, err := os.OpenFile(rcFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	_, err = f.WriteString(line)
	if closeErr := f.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return err
}

// detectTmuxConfigPath resolves the tmux config file path (first match):
//  1. $TMUX_CONFIG env var (if set)
//  2. $XDG_CONFIG_HOME/tmux/tmux.conf (if file exists)
//  3. ~/.tmux.conf (fallback; may not exist; callers create it on write)
//
// Returns "" when the home directory is unavailable and no env vars are set.
func detectTmuxConfigPath() string {
	if v := os.Getenv("TMUX_CONFIG"); v != "" {
		return v
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	xdgConfigHome := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfigHome == "" {
		xdgConfigHome = filepath.Join(home, ".config")
	}

	xdgPath := filepath.Join(xdgConfigHome, "tmux", "tmux.conf")
	if _, err := os.Stat(xdgPath); err == nil {
		return xdgPath
	}

	return filepath.Join(home, ".tmux.conf")
}

// tmuxBindingAlreadyPresent reports whether configPath contains
// "switch-client -t hive". Returns (false, nil) if the file does not exist.
func tmuxBindingAlreadyPresent(configPath string) (bool, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return strings.Contains(string(data), "switch-client -t hive"), nil
}

// appendTmuxBinding appends the bind-key h binding to configPath.
// Creates the file (and parent directories) if they do not exist.
func appendTmuxBinding(configPath string) error {
	const content = "\n# Switch to the hive session\nbind-key h switch-client -t hive\n"

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}

	f, err := os.OpenFile(configPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	_, err = f.WriteString(content)
	if closeErr := f.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return err
}

// defaultConfigPath returns $XDG_CONFIG_HOME/hive/config.yaml.
// This is the write-side counterpart to config.DefaultConfigPath which probes for existing files.
func defaultConfigPath() string {
	return filepath.Join(config.DefaultConfigDir(), "config.yaml")
}

// toStringSlice serialises a []string as a YAML inline sequence using yaml.v3.
// Example: []string{"--foo"} → `["--foo"]`, nil/empty → `[]`.
func toStringSlice(ss []string) string {
	if len(ss) == 0 {
		return "[]"
	}
	node := &yaml.Node{
		Kind:  yaml.SequenceNode,
		Style: yaml.FlowStyle,
	}
	for _, s := range ss {
		node.Content = append(node.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: s,
		})
	}
	out, err := yaml.Marshal(node)
	if err != nil {
		return "[]"
	}
	return strings.TrimRight(string(out), "\n")
}

// toYAMLScalar serialises a string as a safe YAML scalar via yaml.v3.
// Values containing YAML-significant characters (e.g. ": ") are quoted.
func toYAMLScalar(s string) string {
	out, err := yaml.Marshal(s)
	if err != nil {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return strings.TrimRight(string(out), "\n")
}

// renderConfigTemplate executes configTemplate with data and returns the rendered string.
func renderConfigTemplate(data configTemplateData) (string, error) {
	funcMap := template.FuncMap{
		"toStringSlice": toStringSlice,
		"toYAMLScalar":  toYAMLScalar,
	}
	tmpl, err := template.New("config").Funcs(funcMap).Parse(configTemplate)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		return "", err
	}
	return sb.String(), nil
}

const configTemplate = `# hive configuration - generated by ` + "`hive init`" + `
# Full reference: https://hive.colonyops.io/configuration
version: {{ .Version }}

# Parent directories hive scans for git repositories (used in session picker).
# Add folders that contain repo directories, for example ~/code, not an individual repo.
workspaces:
  - {{ .Workspace | toYAMLScalar }}  # add more parent folders here as needed

# AI agent configuration
agents:
  default: {{ .Default }}
{{- range .Agents }}
  {{ .Name }}:
    command: {{ .Name }}
    flags: {{ .Flags | toStringSlice }}
{{- end }}

# tmux integration - enables real-time status monitoring in the TUI
tmux:
  enabled: [tmux]
  poll_interval: 500ms
  # Pane content patterns that indicate an active AI agent window:
  preview_window_matcher: [claude, aider, codex, opencode, agent, amp]

# Session lifecycle rules (last-match wins on remote URL pattern).
rules:
  - pattern: ""  # catch-all
    max_recycled: 3
    commands:
      - hive ctx init  # create .hive symlink in each new session
`

// InitCmd implements the interactive setup wizard.
type InitCmd struct {
	app *hive.App
}

// NewInitCmd constructs an InitCmd.
func NewInitCmd(_ *Flags, app *hive.App) *InitCmd {
	return &InitCmd{app: app}
}

// Register adds the init subcommand to the root CLI command.
func (cmd *InitCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:        "init",
		Usage:       "Run the interactive setup wizard",
		UsageText:   "hive init",
		Description: "interactive setup wizard for new hive installations",
		Action:      cmd.run,
	})
	return app
}

func huhThemeTokyoNight(isDark bool) *huh.Styles {
	t := huh.ThemeBase(isDark)

	primary := styles.ColorPrimary       // #7aa2f7 blue
	secondary := styles.ColorSecondary   // #7dcfff cyan
	foreground := styles.ColorForeground // #c0caf5 light lavender
	muted := styles.ColorMuted           // #565f89 gray-blue
	background := styles.ColorBackground // #1a1b26 dark
	surface := styles.ColorSurface       // #3b4261 selection
	success := styles.ColorSuccess       // #9ece6a green
	warning := styles.ColorWarning       // #e0af68 yellow
	err := styles.ColorError             // #f7768e red

	t.Focused.Base = t.Focused.Base.BorderForeground(primary)
	t.Focused.Card = t.Focused.Base
	// Title uses lavender, not blue; blue is reserved for the border so focus is clear.
	t.Focused.Title = t.Focused.Title.Foreground(foreground).Bold(true)
	t.Focused.NoteTitle = t.Focused.NoteTitle.Foreground(foreground).Bold(true)
	t.Focused.Description = t.Focused.Description.UnsetForeground().Faint(true).Italic(true)
	t.Focused.ErrorIndicator = t.Focused.ErrorIndicator.Foreground(err)
	t.Focused.ErrorMessage = t.Focused.ErrorMessage.Foreground(err)
	// Yellow selector so it reads as a distinct cursor, not just more blue.
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(warning)
	t.Focused.NextIndicator = t.Focused.NextIndicator.Foreground(warning)
	t.Focused.PrevIndicator = t.Focused.PrevIndicator.Foreground(warning)
	t.Focused.Option = t.Focused.Option.UnsetForeground()
	t.Focused.MultiSelectSelector = t.Focused.MultiSelectSelector.Foreground(warning)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(success).Bold(true)
	t.Focused.SelectedPrefix = t.Focused.SelectedPrefix.Foreground(success)
	t.Focused.UnselectedOption = t.Focused.UnselectedOption.UnsetForeground()
	t.Focused.UnselectedPrefix = t.Focused.UnselectedPrefix.Foreground(muted)
	t.Focused.Directory = t.Focused.Directory.Foreground(secondary)
	t.Focused.File = t.Focused.File.Foreground(foreground)
	t.Focused.FocusedButton = t.Focused.FocusedButton.Foreground(background).Background(primary).Bold(true)
	t.Focused.BlurredButton = t.Focused.BlurredButton.Foreground(foreground).Background(surface)
	t.Focused.Next = t.Focused.FocusedButton
	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(warning)
	t.Focused.TextInput.Placeholder = t.Focused.TextInput.Placeholder.Foreground(muted)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(primary)

	// Blurred: visually subordinate with muted titles, hidden cursor, dimmed options.
	t.Blurred = t.Focused
	t.Blurred.Base = t.Blurred.Base.BorderStyle(lipgloss.HiddenBorder())
	t.Blurred.Card = t.Blurred.Base
	t.Blurred.Title = t.Blurred.Title.Foreground(muted).Bold(false)
	t.Blurred.NoteTitle = t.Blurred.NoteTitle.Foreground(muted).Bold(false)
	t.Blurred.Description = t.Blurred.Description.UnsetForeground().Faint(true).Italic(false)
	t.Blurred.SelectSelector = lipgloss.NewStyle().SetString("  ")
	t.Blurred.MultiSelectSelector = lipgloss.NewStyle().SetString("  ")
	t.Blurred.Option = t.Blurred.Option.Foreground(muted)
	t.Blurred.SelectedOption = t.Blurred.SelectedOption.Foreground(foreground).Bold(false)
	t.Blurred.SelectedPrefix = t.Blurred.SelectedPrefix.Foreground(muted)
	t.Blurred.UnselectedOption = t.Blurred.UnselectedOption.Foreground(muted)
	t.Blurred.UnselectedPrefix = t.Blurred.UnselectedPrefix.Foreground(muted)
	t.Blurred.Directory = t.Blurred.Directory.Foreground(muted)
	t.Blurred.File = t.Blurred.File.Foreground(muted)
	t.Blurred.FocusedButton = t.Blurred.FocusedButton.Foreground(foreground).Background(surface).Bold(false)
	t.Blurred.NextIndicator = lipgloss.NewStyle()
	t.Blurred.PrevIndicator = lipgloss.NewStyle()

	return t
}

// expandTilde replaces a leading ~ or ~/ with the user's home directory.
// Returns the path unchanged if ~ cannot be resolved or is not present.
func expandTilde(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return path
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// dirSuggestions returns absolute paths of subdirectories matching the typed input.
// A trailing slash in input lists contents of the expanded directory; otherwise it
// lists siblings whose name starts with the basename of the expanded input.
// Hidden directories (dot-prefixed) are excluded.
func dirSuggestions(input string) []string {
	if input == "" {
		return nil
	}
	expanded := expandTilde(input)

	var dir, prefix string
	if strings.HasSuffix(input, "/") {
		dir = expanded
		prefix = ""
	} else {
		dir = filepath.Dir(expanded)
		prefix = filepath.Base(expanded)
		if prefix == "." {
			prefix = ""
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var result []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if prefix != "" && !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		result = append(result, filepath.Join(dir, e.Name()))
	}
	return result
}

func validateWorkspaceParent(s string) error {
	path := expandTilde(s)
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("path does not exist")
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory")
	}
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return fmt.Errorf("choose the parent folder that contains your repositories, not a git repository")
	}
	return nil
}

func printBanner() {
	primary := lipgloss.NewStyle().Foreground(styles.ColorPrimary).Bold(true)
	muted := lipgloss.NewStyle().Foreground(styles.ColorMuted)
	fg := lipgloss.NewStyle().Foreground(styles.ColorForeground)

	const logo = "" +
		"  ██╗  ██╗██╗██╗   ██╗███████╗\n" +
		"  ██║  ██║██║██║   ██║██╔════╝\n" +
		"  ███████║██║██║   ██║█████╗  \n" +
		"  ██╔══██║██║╚██╗ ██╔╝██╔══╝  \n" +
		"  ██║  ██║██║ ╚████╔╝ ███████╗\n" +
		"  ╚═╝  ╚═╝╚═╝  ╚═══╝  ╚══════╝"

	fmt.Println()
	fmt.Println(primary.Render(logo))
	fmt.Println()
	fmt.Println(muted.Render("  Isolated AI agent session manager"))
	fmt.Println()
	fmt.Println(fg.Render("  This wizard will configure:"))
	fmt.Println(muted.Render("    •  hv alias      - launch hive via tmux"))
	fmt.Println(muted.Render("    •  config.yaml   - workspace & agent settings"))
	fmt.Println(muted.Render("    •  tmux binding  - prefix+H to jump to hive"))
	fmt.Println()
}

func (cmd *InitCmd) run(_ context.Context, _ *cli.Command) error {
	printBanner()

	// ── Pre-flight checks ──────────────────────────────────────────────────
	startDir := "/"
	if h, err := os.UserHomeDir(); err == nil {
		startDir = h
	}

	shellName, rcFile := detectShell()
	aliasAlready := false
	if shellName != "unknown" && rcFile != "" {
		aliasAlready, _ = aliasAlreadyPresent(rcFile, "hv")
	}
	needsAlias := shellName != "unknown" && rcFile != "" && !aliasAlready

	cfgPath := defaultConfigPath()
	needsConfig := true
	if _, err := os.Stat(cfgPath); err == nil {
		needsConfig = false
	}

	hasTmux := false
	if _, err := exec.LookPath("tmux"); err == nil {
		hasTmux = true
	}
	tmuxCfgPath := ""
	tmuxAlready := false
	if hasTmux {
		tmuxCfgPath = detectTmuxConfigPath()
		if tmuxCfgPath != "" {
			tmuxAlready, _ = tmuxBindingAlreadyPresent(tmuxCfgPath)
		}
	}
	needsTmux := hasTmux && tmuxCfgPath != "" && !tmuxAlready

	installed := detectInstalledAgents(knownAgents)
	if len(installed) == 0 {
		installed = []string{"claude"}
	}

	// ── Build unified form ─────────────────────────────────────────────────
	var (
		workspace = startDir
		agentName = installed[0]
		skipPerms = false
		doAlias   = needsAlias
		doTmux    = needsTmux
	)

	// Custom keymap: tab and → accept autocomplete; enter advances the field.
	km := huh.NewDefaultKeyMap()
	km.Input.AcceptSuggestion = key.NewBinding(
		key.WithKeys("tab", "right"),
		key.WithHelp("tab/→", "complete"),
	)
	km.Input.Next = key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "next"),
	)

	var groups []*huh.Group

	if needsConfig {
		agentOpts := make([]huh.Option[string], len(installed))
		for i, a := range installed {
			agentOpts[i] = huh.NewOption(a, a)
		}
		var configFields []huh.Field
		configFields = append(configFields,
			huh.NewInput().
				Title("Repository parent folder").
				Description("Choose the folder that contains your Git repository folders, for example ~/code. Do not choose an individual repo like ~/code/my-app.").
				Value(&workspace).
				SuggestionsFunc(func() []string { return dirSuggestions(workspace) }, &workspace).
				Validate(validateWorkspaceParent),
			huh.NewSelect[string]().
				Title("Default AI agent").
				Options(agentOpts...).
				Value(&agentName),
			huh.NewConfirm().
				Title("Enable skip-permissions mode?").
				Description("Launches the agent without permission prompts").
				Value(&skipPerms),
		)
		groups = append(groups, huh.NewGroup(configFields...))
	}

	if needsAlias || needsTmux {
		var sysFields []huh.Field
		if needsAlias {
			sysFields = append(sysFields, huh.NewConfirm().
				Title(fmt.Sprintf("Append alias hv to %s?", rcFile)).
				Value(&doAlias))
		}
		if needsTmux {
			sysFields = append(sysFields, huh.NewConfirm().
				Title(fmt.Sprintf("Append bind-key h to %s?", tmuxCfgPath)).
				Value(&doTmux))
		}
		groups = append(groups, huh.NewGroup(sysFields...))
	}

	if len(groups) > 0 {
		form := huh.NewForm(groups...).
			WithTheme(huh.ThemeFunc(huhThemeTokyoNight)).
			WithKeyMap(km)
		if err := form.Run(); errors.Is(err, huh.ErrUserAborted) {
			return nil
		}
	}

	workspace = expandTilde(workspace)

	// ── Apply results ──────────────────────────────────────────────────────
	var results []stepResult

	// Shell alias
	switch {
	case shellName == "unknown":
		results = append(results, stepResult{
			name:   "Shell alias",
			status: statusSkipped,
			detail: "unknown shell - add manually: alias hv='tmux new-session -As hive hive'",
		})
	case rcFile == "":
		results = append(results, stepResult{name: "Shell alias", status: statusFailed, detail: "cannot determine home directory"})
	case aliasAlready:
		results = append(results, stepResult{name: "Shell alias", status: statusSkipped, detail: "alias hv already present in " + rcFile})
	case !doAlias:
		results = append(results, stepResult{name: "Shell alias", status: statusSkipped, detail: "skipped"})
	default:
		if err := appendAlias(rcFile, shellName); err != nil {
			results = append(results, stepResult{
				name:    "Shell alias",
				status:  statusFailed,
				detail:  err.Error(),
				fixHint: "chmod u+w " + rcFile + " && hive init",
			})
		} else {
			results = append(results, stepResult{name: "Shell alias", status: statusDone, detail: "appended to " + rcFile})
		}
	}

	// Config file
	if !needsConfig {
		results = append(results, stepResult{name: "Config file", status: statusSkipped, detail: "already exists at " + cfgPath})
	} else {
		results = append(results, cmd.applyConfigFile(cfgPath, agentName, installed, workspace, skipPerms))
	}

	// Tmux binding
	switch {
	case !hasTmux:
		results = append(results, stepResult{name: "Tmux binding", status: statusSkipped, detail: "tmux not found on PATH"})
	case tmuxCfgPath == "":
		results = append(results, stepResult{name: "Tmux binding", status: statusFailed, detail: "cannot determine tmux config path: home directory unavailable"})
	case tmuxAlready:
		results = append(results, stepResult{name: "Tmux binding", status: statusSkipped, detail: "binding already present in " + tmuxCfgPath})
	case !doTmux:
		results = append(results, stepResult{name: "Tmux binding", status: statusSkipped, detail: "skipped"})
	default:
		if err := appendTmuxBinding(tmuxCfgPath); err != nil {
			results = append(results, stepResult{name: "Tmux binding", status: statusFailed, detail: err.Error()})
		} else {
			results = append(results, stepResult{name: "Tmux binding", status: statusDone, detail: "appended to " + tmuxCfgPath})
		}
	}

	printSummary(os.Stderr, results)
	return nil
}

func (cmd *InitCmd) applyConfigFile(cfgPath, agentName string, installed []string, workspace string, skipPerms bool) stepResult {
	agents := make([]agentTemplateData, 0, len(installed))
	agents = append(agents, agentTemplateData{
		Name:  agentName,
		Flags: flagsFor(agentName, skipPerms),
	})
	for _, name := range installed {
		if name == agentName {
			continue
		}
		agents = append(agents, agentTemplateData{
			Name:  name,
			Flags: flagsFor(name, skipPerms),
		})
	}

	data := configTemplateData{
		Version:   cmd.app.Build.Version,
		Workspace: workspace,
		Default:   agentName,
		Agents:    agents,
	}
	rendered, err := renderConfigTemplate(data)
	if err != nil {
		return stepResult{name: "Config file", status: statusFailed, detail: err.Error()}
	}

	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		return stepResult{name: "Config file", status: statusFailed, detail: err.Error()}
	}

	tmp := cfgPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(rendered), 0o644); err != nil {
		return stepResult{name: "Config file", status: statusFailed, detail: err.Error()}
	}
	if err := os.Rename(tmp, cfgPath); err != nil {
		_ = os.Remove(tmp)
		return stepResult{name: "Config file", status: statusFailed, detail: err.Error()}
	}
	return stepResult{name: "Config file", status: statusDone, detail: "created " + cfgPath}
}

// flagsFor returns the skip-permissions flags for an agent when skipPerms is true,
// or nil if the agent has no known flags or skipPerms is false.
func flagsFor(agent string, skipPerms bool) []string {
	if !skipPerms {
		return nil
	}
	return agentFlagMap[agent]
}

func printSummary(w io.Writer, results []stepResult) {
	divider := styles.TextMutedStyle.Render(strings.Repeat("─", 54))
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Setup Summary")
	_, _ = fmt.Fprintln(w, divider)
	for _, r := range results {
		var icon string
		switch r.status {
		case statusDone:
			icon = styles.TextSuccessStyle.Render("✓")
		case statusSkipped:
			icon = styles.TextMutedStyle.Render("-")
		case statusFailed:
			icon = styles.TextErrorStyle.Render("✗")
		}
		_, _ = fmt.Fprintf(w, "  %s  %-18s %s\n", icon, r.name, r.detail)
		if r.fixHint != "" {
			_, _ = fmt.Fprintf(w, "     Fix: %s\n", r.fixHint)
		}
	}
	_, _ = fmt.Fprintln(w, divider)
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, styles.TextMutedStyle.Render("Run 'hive doctor' to verify your full setup."))
}
