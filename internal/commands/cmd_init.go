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

	"charm.land/huh/v2"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/hive"
	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"
)

type configTemplateData struct {
	Version      string
	Workspace    string
	AgentDefault string
	AgentFlags   []string
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
	aborted bool
}

var knownAgents = []string{"claude", "opencode", "codex", "pi", "amp"}

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
//  3. ~/.tmux.conf (fallback; may not exist — callers create it on write)
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

// appendTmuxBinding appends the bind-key H binding to configPath.
// Creates the file (and parent directories) if they do not exist.
func appendTmuxBinding(configPath string) error {
	const content = "\n# Switch to the hive session\nbind-key H switch-client -t hive\n"

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
// This is the write-side counterpart to DefaultConfigPath in flags.go which probes for existing files.
func defaultConfigPath() string {
	return filepath.Join(defaultConfigDir(), "config.yaml")
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

const configTemplate = `# hive configuration — generated by ` + "`hive init`" + `
# Full reference: https://hive.colonyops.io/configuration
version: {{ .Version }}

# Directories hive scans for git repositories (used in session picker).
workspaces:
  - {{ .Workspace | toYAMLScalar }}  # add more paths here as needed

# Context directory settings
context:
  base_dir: "${XDG_DATA_HOME:-$HOME/.local/share}/hive/context"

# AI agent configuration
agents:
  default: {{ .AgentDefault }}
  {{ .AgentDefault }}:
    command: {{ .AgentDefault }}
    flags: {{ .AgentFlags | toStringSlice }}

# tmux integration — enables real-time status monitoring in the TUI
tmux:
  enabled: [tmux]
  poll_interval: 500ms
  # Pane content patterns that indicate an active AI agent window:
  preview_window_matcher: [claude, aider, codex, opencode, agent, pi, amp]

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

func (cmd *InitCmd) run(_ context.Context, _ *cli.Command) error {
	var results []stepResult
	for _, step := range []func() stepResult{
		cmd.stepShellAlias,
		cmd.stepConfigFile,
		cmd.stepTmuxBinding,
	} {
		r := step()
		results = append(results, r)
		if r.aborted {
			break
		}
	}
	printSummary(os.Stderr, results)
	return nil
}

func (cmd *InitCmd) stepShellAlias() stepResult {
	shellName, rcFile := detectShell()
	if shellName == "unknown" {
		return stepResult{
			name:   "Shell alias",
			status: statusSkipped,
			detail: "unknown shell — add manually: alias hv='tmux new-session -As hive hive'",
		}
	}
	if rcFile == "" {
		return stepResult{name: "Shell alias", status: statusFailed, detail: "cannot determine home directory"}
	}

	present, err := aliasAlreadyPresent(rcFile, "hv")
	if err != nil {
		return stepResult{name: "Shell alias", status: statusFailed, detail: err.Error()}
	}
	if present {
		return stepResult{name: "Shell alias", status: statusSkipped, detail: "alias hv already present in " + rcFile}
	}

	var confirm bool
	err = huh.NewConfirm().
		Title(fmt.Sprintf("Append alias hv to %s?", rcFile)).
		Value(&confirm).
		Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return stepResult{name: "Shell alias", status: statusFailed, detail: "wizard aborted", aborted: true}
	}
	if !confirm {
		return stepResult{name: "Shell alias", status: statusSkipped, detail: "skipped"}
	}

	if err := appendAlias(rcFile, shellName); err != nil {
		return stepResult{name: "Shell alias", status: statusFailed, detail: err.Error(), fixHint: "chmod u+w " + rcFile + " && hive init"}
	}
	return stepResult{name: "Shell alias", status: statusDone, detail: "appended to " + rcFile}
}

func (cmd *InitCmd) stepConfigFile() stepResult {
	cfgPath := defaultConfigPath()
	if _, err := os.Stat(cfgPath); err == nil {
		return stepResult{name: "Config file", status: statusSkipped, detail: "already exists at " + cfgPath}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return stepResult{name: "Config file", status: statusFailed, detail: "cannot determine home directory: " + err.Error()}
	}

	workspace, aborted := selectWorkspace(home)
	if aborted {
		return stepResult{name: "Config file", status: statusFailed, detail: "wizard aborted", aborted: true}
	}

	agentDefault, agentFlags, aborted := selectAgent()
	if aborted {
		return stepResult{name: "Config file", status: statusFailed, detail: "wizard aborted", aborted: true}
	}

	data := configTemplateData{
		Version:      cmd.app.Build.Version,
		Workspace:    workspace,
		AgentDefault: agentDefault,
		AgentFlags:   agentFlags,
	}
	rendered, err := renderConfigTemplate(data)
	if err != nil {
		return stepResult{name: "Config file", status: statusFailed, detail: err.Error()}
	}

	var writeConfirm bool
	if err := huh.NewConfirm().
		Title(fmt.Sprintf("Write config to %s?", cfgPath)).
		Value(&writeConfirm).
		Run(); errors.Is(err, huh.ErrUserAborted) {
		return stepResult{name: "Config file", status: statusFailed, detail: "wizard aborted", aborted: true}
	}
	if !writeConfirm {
		return stepResult{name: "Config file", status: statusSkipped, detail: "skipped"}
	}

	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		return stepResult{name: "Config file", status: statusFailed, detail: err.Error()}
	}

	// atomic write via rename
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

func (cmd *InitCmd) stepTmuxBinding() stepResult {
	if _, err := exec.LookPath("tmux"); err != nil {
		return stepResult{name: "Tmux binding", status: statusSkipped, detail: "tmux not found on PATH"}
	}

	cfgPath := detectTmuxConfigPath()
	if cfgPath == "" {
		return stepResult{name: "Tmux binding", status: statusFailed, detail: "cannot determine tmux config path: home directory unavailable"}
	}

	present, err := tmuxBindingAlreadyPresent(cfgPath)
	if err != nil {
		return stepResult{name: "Tmux binding", status: statusFailed, detail: err.Error()}
	}
	if present {
		return stepResult{name: "Tmux binding", status: statusSkipped, detail: "binding already present in " + cfgPath}
	}

	var confirm bool
	if err := huh.NewConfirm().
		Title(fmt.Sprintf("Append bind-key H switch-client -t hive to %s?", cfgPath)).
		Value(&confirm).
		Run(); errors.Is(err, huh.ErrUserAborted) {
		return stepResult{name: "Tmux binding", status: statusFailed, detail: "wizard aborted", aborted: true}
	}
	if !confirm {
		return stepResult{name: "Tmux binding", status: statusSkipped, detail: "skipped"}
	}

	if err := appendTmuxBinding(cfgPath); err != nil {
		return stepResult{name: "Tmux binding", status: statusFailed, detail: err.Error()}
	}
	return stepResult{name: "Tmux binding", status: statusDone, detail: "appended to " + cfgPath}
}

// selectWorkspace prompts the user to choose a workspace directory.
// Falls back to filepath.Join(home, "code") on skip or picker error.
// Returns (path, true) if the user aborted the wizard.
func selectWorkspace(home string) (path string, aborted bool) {
	var pick bool
	if err := huh.NewConfirm().
		Title("Select a workspace directory for your git repositories?").
		Value(&pick).
		Run(); errors.Is(err, huh.ErrUserAborted) {
		return "", true
	}
	if pick {
		var picked string
		err := huh.NewFilePicker().
			Title("Select your workspace directory").
			Description("The folder that contains your git repositories").
			DirAllowed(true).
			FileAllowed(false).
			CurrentDirectory(home).
			Value(&picked).
			Run()
		if err == nil && picked != "" {
			return picked, false
		}
	}
	return filepath.Join(home, "code"), false
}

// selectAgent prompts for the default AI agent and optional skip-permissions flags.
// Returns ("claude", nil, false) if no known agents are installed.
// Returns ("", nil, true) if the user aborted the wizard.
func selectAgent() (name string, agentFlags []string, aborted bool) {
	installed := detectInstalledAgents(knownAgents)
	if len(installed) == 0 {
		return "claude", nil, false
	}

	opts := make([]huh.Option[string], len(installed))
	for i, a := range installed {
		opts[i] = huh.NewOption(a, a)
	}

	name = "claude"
	var selected string
	if err := huh.NewSelect[string]().
		Title("Select your default AI agent").
		Options(opts...).
		Value(&selected).
		Run(); errors.Is(err, huh.ErrUserAborted) {
		return "", nil, true
	}
	if selected != "" {
		name = selected
	}

	if flags, ok := agentFlagMap[name]; ok {
		var skipPerms bool
		if err := huh.NewConfirm().
			Title(fmt.Sprintf("Run %s in skip-permissions mode?", name)).
			Value(&skipPerms).
			Run(); errors.Is(err, huh.ErrUserAborted) {
			return "", nil, true
		}
		if skipPerms {
			agentFlags = flags
		}
	}
	return name, agentFlags, false
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
