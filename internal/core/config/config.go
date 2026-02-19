// Package config handles configuration loading and validation for hive.
package config

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/colonyops/hive/internal/core/action"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/hay-kot/criterio"
	"gopkg.in/yaml.v3"
)

// ParseExitCondition evaluates an exit condition string.
// If the string starts with $, it checks the env var value.
// Otherwise it parses the string directly as a boolean.
// Returns false if parsing fails or env var is unset.
func ParseExitCondition(s string) bool {
	if s == "" {
		return false
	}
	if envVar, ok := strings.CutPrefix(s, "$"); ok {
		s = os.Getenv(envVar)
	}
	result, _ := strconv.ParseBool(s)
	return result
}

// Form field type constants.
const (
	FormTypeText        = "text"
	FormTypeTextArea    = "textarea"
	FormTypeSelect      = "select"
	FormTypeMultiSelect = "multi-select"
)

// Form preset constants.
const (
	FormPresetSessionSelector = "SessionSelector"
	FormPresetProjectSelector = "ProjectSelector"
)

// Form filter constants for SessionSelector preset.
const (
	FormFilterActive = "active" // Only active sessions (default)
	FormFilterAll    = "all"    // All sessions regardless of state
)

// ValidSessionFilters lists valid filter values for SessionSelector.
var ValidSessionFilters = []string{FormFilterActive, FormFilterAll}

// ValidFormTypes lists all valid form field types.
var ValidFormTypes = []string{FormTypeText, FormTypeTextArea, FormTypeSelect, FormTypeMultiSelect}

// ValidFormPresets lists all valid form field presets.
var ValidFormPresets = []string{FormPresetSessionSelector, FormPresetProjectSelector}

// FormField defines an input field in a UserCommand form.
type FormField struct {
	Variable    string   `yaml:"variable"`              // Template variable name (under .Form)
	Type        string   `yaml:"type,omitempty"`        // text, textarea, select, multi-select
	Preset      string   `yaml:"preset,omitempty"`      // SessionSelector, ProjectSelector
	Label       string   `yaml:"label"`                 // Display label
	Placeholder string   `yaml:"placeholder,omitempty"` // Input placeholder text
	Default     string   `yaml:"default,omitempty"`     // Default value (text/textarea/select)
	Options     []string `yaml:"options,omitempty"`     // Static options for select/multi-select
	Multi       bool     `yaml:"multi,omitempty"`       // For presets: enable multi-select
	Filter      string   `yaml:"filter,omitempty"`      // For SessionSelector: "active" (default) or "all"
}

// defaultUserCommands provides built-in commands that users can override.
var defaultUserCommands = map[string]UserCommand{
	"Recycle": {
		Action:  action.TypeRecycle,
		Help:    "recycle",
		Confirm: "Are you sure you want to recycle this session?",
	},
	"Delete": {
		Action:  action.TypeDelete,
		Help:    "delete",
		Confirm: "Are you sure you want to delete this session?",
	},
	"NewSession": {
		Action: action.TypeNewSession,
		Help:   "new session",
		Silent: true,
	},
	"DocReview": {
		Action: action.TypeDocReview,
		Help:   "review documents",
		Silent: true,
	},
	"FilterAll": {
		Action: action.TypeFilterAll,
		Help:   "show all sessions",
		Silent: true,
	},
	"FilterActive": {
		Action: action.TypeFilterActive,
		Help:   "show sessions with active agents",
		Silent: true,
	},
	"FilterApproval": {
		Action: action.TypeFilterApproval,
		Help:   "show sessions needing approval",
		Silent: true,
	},
	"FilterReady": {
		Action: action.TypeFilterReady,
		Help:   "show sessions with idle agents",
		Silent: true,
	},
	"ThemePreview": {
		Action: action.TypeSetTheme,
		Help:   "preview theme (" + strings.Join(styles.ThemeNames(), ", ") + ")",
		Silent: true,
	},
	"Messages": {
		Action: action.TypeMessages,
		Help:   "show notification history",
		Silent: true,
	},
	"RenameSession": {
		Action: action.TypeRenameSession,
		Help:   "rename session",
		Silent: true,
	},
	"NextActive": {
		Action: action.TypeNextActive,
		Help:   "next active session",
		Silent: true,
	},
	"PrevActive": {
		Action: action.TypePrevActive,
		Help:   "prev active session",
		Silent: true,
	},
	"SendBatch": {
		Sh: `{{ range .Form.targets }}
{{ agentSend }} {{ .Name | shq }}:claude {{ $.Form.message | shq }}
{{ end }}`,
		Form: []FormField{
			{
				Variable: "targets",
				Preset:   FormPresetSessionSelector,
				Multi:    true,
				Label:    "Select recipients",
			},
			{
				Variable:    "message",
				Type:        FormTypeText,
				Label:       "Message",
				Placeholder: "Type your message...",
			},
		},
		Help:   "send message to multiple agents",
		Silent: true,
	},
}

// defaultKeybindings provides built-in keybindings that users can override.
var defaultKeybindings = map[string]Keybinding{
	"r":      {Cmd: "Recycle"},
	"d":      {Cmd: "Delete"},
	"n":      {Cmd: "NewSession"},
	"enter":  {Cmd: "TmuxOpen"},
	"ctrl+d": {Cmd: "TmuxKill"},
	"A":      {Cmd: "AgentSend"},
	"p":      {Cmd: "TmuxPopUp"},
	"R":      {Cmd: "RenameSession"},
	"J":      {Cmd: "NextActive"},
	"K":      {Cmd: "PrevActive"},
}

// CurrentConfigVersion is the latest config schema version.
// Increment this when making breaking changes to config format.
const CurrentConfigVersion = "0.2.5"

// Config holds the application configuration.
type Config struct {
	Version             string                 `yaml:"version"`
	CopyCommand         string                 `yaml:"copy_command"` // command to copy to clipboard (e.g., pbcopy, xclip)
	Git                 GitConfig              `yaml:"git"`
	GitPath             string                 `yaml:"git_path"`
	Keybindings         map[string]Keybinding  `yaml:"keybindings"`
	UserCommands        map[string]UserCommand `yaml:"usercommands"`
	Rules               []Rule                 `yaml:"rules"`
	Agents              AgentsConfig           `yaml:"agents"`
	AutoDeleteCorrupted bool                   `yaml:"auto_delete_corrupted"`
	History             HistoryConfig          `yaml:"history"`
	Context             ContextConfig          `yaml:"context"`
	TUI                 TUIConfig              `yaml:"tui"`
	Review              ReviewConfig           `yaml:"review"`
	Messaging           MessagingConfig        `yaml:"messaging"`
	Tmux                TmuxConfig             `yaml:"tmux"`
	Database            DatabaseConfig         `yaml:"database"`
	Plugins             PluginsConfig          `yaml:"plugins"`
	RepoDirs            []string               `yaml:"repo_dirs"` // directories containing git repositories for new session dialog
	DataDir             string                 `yaml:"-"`         // set by caller, not from config file
}

// AgentsConfig holds agent profile configuration.
// The "default" key selects which profile to use; all other keys are profile definitions.
type AgentsConfig struct {
	Default  string                  // reserved key selecting the active profile
	Profiles map[string]AgentProfile // profile name → agent configuration
}

// UnmarshalYAML separates the "default" key from profile entries.
func (a *AgentsConfig) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("agents: expected mapping, got %v", node.Kind)
	}

	a.Profiles = make(map[string]AgentProfile)

	for i := 0; i < len(node.Content)-1; i += 2 {
		key := node.Content[i].Value
		val := node.Content[i+1]

		if key == "default" {
			var s string
			if err := val.Decode(&s); err != nil {
				return fmt.Errorf("agents.default: %w", err)
			}
			a.Default = s
			continue
		}

		var profile AgentProfile
		if err := val.Decode(&profile); err != nil {
			return fmt.Errorf("agents.%s: %w", key, err)
		}
		a.Profiles[key] = profile
	}

	return nil
}

// DefaultProfile returns the profile selected by the Default key.
// Returns a zero AgentProfile if the default is not found.
func (a AgentsConfig) DefaultProfile() AgentProfile {
	if p, ok := a.Profiles[a.Default]; ok {
		return p
	}
	return AgentProfile{}
}

// AgentProfile defines an agent's command and flags.
type AgentProfile struct {
	Command string   `yaml:"command"` // CLI binary (defaults to profile key if omitted)
	Flags   []string `yaml:"flags"`   // extra CLI args appended to command on spawn
}

// CommandOrDefault returns the command, falling back to the given key name.
func (p AgentProfile) CommandOrDefault(key string) string {
	if p.Command != "" {
		return p.Command
	}
	return key
}

// ShellFlags returns the flags as a space-joined string.
// Individual flags are NOT quoted — the caller is responsible for quoting
// the entire value (e.g., via shq in templates) or relying on shell word
// splitting when consuming the value.
func (p AgentProfile) ShellFlags() string {
	return strings.Join(p.Flags, " ")
}

// HistoryConfig holds command history configuration.
type HistoryConfig struct {
	MaxEntries int `yaml:"max_entries"`
}

// ContextConfig configures context directory behavior.
type ContextConfig struct {
	SymlinkName string `yaml:"symlink_name"` // default: ".hive"
}

// TUIConfig holds TUI-related configuration.
type TUIConfig struct {
	Theme           string        `yaml:"theme"`            // built-in theme name (default: "tokyo-night")
	RefreshInterval time.Duration `yaml:"refresh_interval"` // default: 15s, 0 to disable
	PreviewEnabled  bool          `yaml:"preview_enabled"`  // enable tmux pane preview sidebar
	Icons           *bool         `yaml:"icons"`            // enable nerd font icons (nil = true by default)
	Preview         PreviewConfig `yaml:"preview"`          // preview panel configuration
	Views           ViewsConfig   `yaml:"views"`            // toggle optional TUI tabs
}

// ViewsConfig controls which optional TUI tabs are enabled.
// All optional views default to disabled.
type ViewsConfig struct {
	Store bool `yaml:"store"` // KV store browser (default: false)
}

// ReviewConfig holds review-related configuration.
type ReviewConfig struct {
	CommentLineWidth int `yaml:"comment_line_width"` // minimum width for comment wrapping (default: 80)
}

// CommentLineWidthOrDefault returns the configured comment line width or 80 if not set.
func (r ReviewConfig) CommentLineWidthOrDefault() int {
	if r.CommentLineWidth <= 0 {
		return 80
	}
	return r.CommentLineWidth
}

// IconsEnabled returns true if nerd font icons should be shown.
func (t TUIConfig) IconsEnabled() bool {
	return t.Icons == nil || *t.Icons
}

// PreviewConfig holds preview panel template configuration.
type PreviewConfig struct {
	TitleTemplate  string `yaml:"title_template"`  // Go template for panel title (e.g., "{{ .Name }} • #{{ .ShortID }}")
	StatusTemplate string `yaml:"status_template"` // Go template for status line (e.g., "{{ .Icon.GitBranch }} {{ .Branch }}")
}

// MessagingConfig holds messaging-related configuration.
type MessagingConfig struct {
	TopicPrefix string `yaml:"topic_prefix"` // default: "agent"
	MaxMessages int    `yaml:"max_messages"` // max messages per topic (default: 100, 0 = unlimited)
}

// TmuxConfig holds tmux integration configuration.
type TmuxConfig struct {
	PollInterval         time.Duration `yaml:"poll_interval"`          // status check frequency, default 1.5s
	PreviewWindowMatcher []string      `yaml:"preview_window_matcher"` // regex patterns for preferred window names (e.g., ["claude", "aider"])
}

// PluginsConfig holds configuration for the plugin system.
type PluginsConfig struct {
	ShellWorkers int                    `yaml:"shell_workers"` // shared subprocess pool size (default: 5)
	GitHub       GitHubPluginConfig     `yaml:"github"`
	Beads        BeadsPluginConfig      `yaml:"beads"`
	LazyGit      LazyGitPluginConfig    `yaml:"lazygit"`
	Neovim       NeovimPluginConfig     `yaml:"neovim"`
	ContextDir   ContextDirPluginConfig `yaml:"contextdir"`
	Claude       ClaudePluginConfig     `yaml:"claude"`
	Tmux         TmuxPluginConfig       `yaml:"tmux"`
}

// TmuxPluginConfig holds tmux plugin configuration.
type TmuxPluginConfig struct {
	Enabled *bool `yaml:"enabled"` // nil = auto-detect, true/false = override
}

// GitHubPluginConfig holds GitHub plugin configuration.
type GitHubPluginConfig struct {
	Enabled      *bool         `yaml:"enabled"`       // nil = auto-detect, true/false = override
	ResultsCache time.Duration `yaml:"results_cache"` // status cache duration (default: 8m)
}

// BeadsPluginConfig holds Beads plugin configuration.
type BeadsPluginConfig struct {
	Enabled      *bool         `yaml:"enabled"`       // nil = auto-detect, true/false = override
	ResultsCache time.Duration `yaml:"results_cache"` // status cache duration (default: 30s)
}

// LazyGitPluginConfig holds lazygit plugin configuration.
type LazyGitPluginConfig struct {
	Enabled *bool `yaml:"enabled"` // nil = auto-detect, true/false = override
}

// NeovimPluginConfig holds neovim plugin configuration.
type NeovimPluginConfig struct {
	Enabled *bool `yaml:"enabled"` // nil = auto-detect, true/false = override
}

// ContextDirPluginConfig holds context directory plugin configuration.
type ContextDirPluginConfig struct {
	Enabled *bool `yaml:"enabled"` // nil = auto-detect, true/false = override
}

// ClaudePluginConfig holds Claude Code plugin configuration.
type ClaudePluginConfig struct {
	Enabled         *bool         `yaml:"enabled"`          // nil = auto-detect, true/false = override
	CacheTTL        time.Duration `yaml:"cache_ttl"`        // status cache duration (default: 30s)
	YellowThreshold int           `yaml:"yellow_threshold"` // yellow above this % (default: 60)
	RedThreshold    int           `yaml:"red_threshold"`    // red above this % (default: 80)
	ModelLimit      int           `yaml:"model_limit"`      // context limit (default: 200000)
}

// DatabaseConfig holds SQLite database configuration.
type DatabaseConfig struct {
	MaxOpenConns int `yaml:"max_open_conns"` // max open connections (default: 2)
	MaxIdleConns int `yaml:"max_idle_conns"` // max idle connections (default: 2)
	BusyTimeout  int `yaml:"busy_timeout"`   // busy timeout in milliseconds (default: 5000)
}

// GitConfig holds git-related configuration.
type GitConfig struct {
	StatusWorkers int `yaml:"status_workers"`
}

// WindowConfig defines a tmux window to create when spawning a session.
type WindowConfig struct {
	Name    string `yaml:"name"`              // Window name (template string, required)
	Command string `yaml:"command,omitempty"` // Command to run (template string, empty = shell)
	Dir     string `yaml:"dir,omitempty"`     // Working directory override (template string)
	Focus   bool   `yaml:"focus,omitempty"`   // Select this window after creation
}

// Rule defines actions to take for matching repositories.
type Rule struct {
	// Pattern matches against remote URL (regex). Empty = matches all.
	Pattern string `yaml:"pattern"`
	// Commands to run in the session directory after clone/recycle.
	Commands []string `yaml:"commands,omitempty"`
	// Copy are glob patterns to copy from source directory.
	Copy []string `yaml:"copy,omitempty"`
	// MaxRecycled sets the max recycled sessions for matching repos.
	// nil = inherit from previous rule or default (5), 0 = unlimited, >0 = limit
	MaxRecycled *int `yaml:"max_recycled,omitempty"`
	// Windows defines tmux windows to create when spawning a session.
	// Mutually exclusive with Spawn/BatchSpawn.
	Windows []WindowConfig `yaml:"windows,omitempty"`
	// Spawn commands to run when creating a new session (hive new).
	Spawn []string `yaml:"spawn,omitempty"`
	// BatchSpawn commands to run when creating a batch session (hive batch).
	BatchSpawn []string `yaml:"batch_spawn,omitempty"`
	// Recycle commands to run when recycling a session.
	Recycle []string `yaml:"recycle,omitempty"`
}

// Keybinding defines a TUI keybinding that references a UserCommand.
type Keybinding struct {
	Cmd     string `yaml:"cmd"`     // command name (required, references UserCommand)
	Help    string `yaml:"help"`    // optional override for help text
	Confirm string `yaml:"confirm"` // optional override for confirmation prompt
}

// UserCommandOptions controls execution behaviour for UserCommands with windows.
type UserCommandOptions struct {
	// SessionName is a template string. When non-empty, a new Hive session is created
	// with this name before sh: runs and windows are opened.
	SessionName string `yaml:"session_name,omitempty"`
	// Remote overrides the remote URL for new session creation.
	// Only valid when SessionName is also set.
	Remote string `yaml:"remote,omitempty"`
	// Background creates windows without attaching or switching to the tmux session.
	Background bool `yaml:"background,omitempty"`
}

// UserCommand defines a named command accessible via command palette or keybindings.
type UserCommand struct {
	Action  action.Type        `yaml:"action,omitempty"`  // built-in action (Recycle, Delete, etc.) - mutually exclusive with sh
	Sh      string             `yaml:"sh"`                // shell command template - mutually exclusive with action
	Windows []WindowConfig     `yaml:"windows,omitempty"` // Tmux windows to open after sh: completes
	Options UserCommandOptions `yaml:"options,omitempty"` // Execution options for window-based commands
	Form    []FormField        `yaml:"form,omitempty"`    // interactive input fields collected before sh execution
	Help    string             `yaml:"help"`              // description shown in palette/help
	Confirm string             `yaml:"confirm"`           // confirmation prompt (empty = no confirm)
	Silent  bool               `yaml:"silent"`            // skip loading popup for fast commands
	Exit    string             `yaml:"exit"`              // exit hive after command (bool or $ENV_VAR)
	Scope   []string           `yaml:"scope,omitempty"`   // views where command is active (empty = global)
}

// ShouldExit evaluates the Exit condition.
func (u UserCommand) ShouldExit() bool {
	return ParseExitCondition(u.Exit)
}

// UnmarshalYAML supports string shorthand: "cmd" → {sh: "cmd"}
func (u *UserCommand) UnmarshalYAML(node *yaml.Node) error {
	// Try string shorthand first
	if node.Kind == yaml.ScalarNode {
		var sh string
		if err := node.Decode(&sh); err != nil {
			return err
		}
		u.Sh = sh
		return nil
	}

	// Decode as struct (use alias to avoid recursion)
	type userCommandAlias UserCommand
	var alias userCommandAlias
	if err := node.Decode(&alias); err != nil {
		return err
	}
	*u = UserCommand(alias)
	return nil
}

// DefaultWindows returns the default window layout for new sessions.
// Uses template strings rendered at spawn time from the active agent profile.
func DefaultWindows() []WindowConfig {
	return []WindowConfig{
		{
			Name:    "{{ agentWindow }}",
			Command: `{{ agentCommand }} {{ agentFlags }}{{- if .Prompt }} {{ .Prompt }}{{ end }}`,
			Focus:   true,
		},
		{Name: "shell"},
	}
}

// DefaultRecycleCommands are the default commands run when recycling a session.
var DefaultRecycleCommands = []string{
	"git fetch origin",
	"git checkout -f {{ .DefaultBranch }}",
	"git reset --hard origin/{{ .DefaultBranch }}",
	"git clean -fd",
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Git: GitConfig{
			StatusWorkers: 3,
		},
		GitPath:             "git",
		Keybindings:         map[string]Keybinding{},
		AutoDeleteCorrupted: true,
		History: HistoryConfig{
			MaxEntries: 100,
		},
		Context: ContextConfig{
			SymlinkName: ".hive",
		},
		TUI: TUIConfig{
			RefreshInterval: 15 * time.Second,
			PreviewEnabled:  true,
		},
		Messaging: MessagingConfig{
			TopicPrefix: "agent",
			MaxMessages: 100,
		},
	}
}

// Load reads configuration from the given path and sets the data directory.
// If configPath is empty or doesn't exist, returns defaults with the provided dataDir.
func Load(configPath, dataDir string) (*Config, error) {
	cfg := DefaultConfig()
	cfg.DataDir = dataDir

	if configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			data, err := os.ReadFile(configPath)
			if err != nil {
				return nil, fmt.Errorf("read config file: %w", err)
			}

			if err := yaml.Unmarshal(data, &cfg); err != nil {
				return nil, fmt.Errorf("parse config file: %w", err)
			}

			// Re-set dataDir since Unmarshal may have cleared it
			cfg.DataDir = dataDir
		}
	}

	// Validate user keybindings before merging defaults (defaults may reference
	// plugin commands that don't exist at config-load time).
	if err := cfg.validateUserKeybindings(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Merge user keybindings into defaults (user config overrides defaults)
	cfg.Keybindings = mergeKeybindings(defaultKeybindings, cfg.Keybindings)

	// Apply defaults for zero values
	cfg.applyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// applyDefaults sets default values for any unset configuration options.
func (c *Config) applyDefaults() {
	defaults := DefaultConfig()
	if c.Git.StatusWorkers == 0 {
		c.Git.StatusWorkers = defaults.Git.StatusWorkers
	}
	if c.History.MaxEntries == 0 {
		c.History.MaxEntries = defaults.History.MaxEntries
	}
	if c.Context.SymlinkName == "" {
		c.Context.SymlinkName = defaults.Context.SymlinkName
	}
	if c.TUI.Theme == "" {
		c.TUI.Theme = styles.DefaultTheme
	}
	if c.CopyCommand == "" {
		c.CopyCommand = defaultCopyCommand()
	}
	if c.Tmux.PollInterval == 0 {
		c.Tmux.PollInterval = 1500 * time.Millisecond
	}
	if len(c.Tmux.PreviewWindowMatcher) == 0 {
		c.Tmux.PreviewWindowMatcher = []string{"claude", "aider", "codex", "cursor", "crush", "agent", "llm"}
	}
	if c.Database.MaxOpenConns == 0 {
		c.Database.MaxOpenConns = 2
	}
	if c.Database.MaxIdleConns == 0 {
		c.Database.MaxIdleConns = 2
	}
	if c.Database.BusyTimeout == 0 {
		c.Database.BusyTimeout = 5000
	}
	if c.Plugins.ShellWorkers == 0 {
		c.Plugins.ShellWorkers = 5
	}
	if c.Plugins.GitHub.ResultsCache == 0 {
		c.Plugins.GitHub.ResultsCache = 8 * time.Minute
	}
	if c.Plugins.Beads.ResultsCache == 0 {
		c.Plugins.Beads.ResultsCache = 30 * time.Second
	}
	if len(c.Agents.Profiles) == 0 {
		c.Agents.Profiles = map[string]AgentProfile{
			"claude": {},
		}
	}
	if c.Agents.Default == "" {
		c.Agents.Default = "claude"
	}
}

// defaultCopyCommand returns the default clipboard command for the current OS.
func defaultCopyCommand() string {
	switch runtime.GOOS {
	case "darwin":
		return "pbcopy"
	case "windows":
		return "clip"
	default:
		// Linux and others - try xclip first, fall back to xsel
		return "xclip -selection clipboard"
	}
}

// mergeKeybindings merges user keybindings into defaults.
// User keybindings override defaults for the same key.
func mergeKeybindings(defaults, user map[string]Keybinding) map[string]Keybinding {
	result := make(map[string]Keybinding, len(defaults)+len(user))

	// Copy defaults first
	maps.Copy(result, defaults)
	maps.Copy(result, user)

	return result
}

// mergeUserCommands merges user commands into defaults.
// User commands override defaults for the same name.
func mergeUserCommands(defaults, user map[string]UserCommand) map[string]UserCommand {
	result := make(map[string]UserCommand, len(defaults)+len(user))

	// Copy defaults first
	maps.Copy(result, defaults)
	maps.Copy(result, user)

	return result
}

// MergedUserCommands returns user commands merged with system defaults.
// System defaults (Recycle, Delete) can be overridden by user config.
func (c *Config) MergedUserCommands() map[string]UserCommand {
	return mergeUserCommands(defaultUserCommands, c.UserCommands)
}

// DefaultUserCommands returns the built-in system commands.
// Used by plugin manager to properly merge system → plugin → user commands.
func DefaultUserCommands() map[string]UserCommand {
	return defaultUserCommands
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	return criterio.ValidateStruct(
		criterio.Run("git_path", c.GitPath, criterio.Required[string]),
		criterio.Run("data_dir", c.DataDir, criterio.Required[string]),
		criterio.Run("git.status_workers", c.Git.StatusWorkers, criterio.Min(1)),
		criterio.Run("database.max_open_conns", c.Database.MaxOpenConns, criterio.Min(1)),
		criterio.Run("database.max_idle_conns", c.Database.MaxIdleConns, criterio.Min(1)),
		criterio.Run("database.busy_timeout", c.Database.BusyTimeout, criterio.Min(0)),
		c.validateTheme(),
		c.validateKeybindingsBasic(),
		c.validateUserCommandsBasic(),
		c.validateMaxRecycled(),
		c.validateAgents(),
		c.validateWindowsBasic(),
	)
}

// validateUserCommandsBasic performs basic usercommand validation for the Validate() method.
func (c *Config) validateUserCommandsBasic() error {
	var errs criterio.FieldErrorsBuilder
	for name, cmd := range c.UserCommands {
		field := fmt.Sprintf("usercommands[%q]", name)

		// Validate command name format
		if name == "" {
			errs = errs.Append(field, fmt.Errorf("command name cannot be empty"))
			continue
		}
		if !isValidCommandName(name) {
			errs = errs.Append(field, fmt.Errorf("invalid command name: must contain only alphanumeric characters, dashes, and underscores (no spaces)"))
			continue
		}

		// Must have at least one of: action, sh, windows
		hasAction := cmd.Action != ""
		hasSh := cmd.Sh != ""
		hasWindows := len(cmd.Windows) > 0

		if !hasAction && !hasSh && !hasWindows {
			errs = errs.Append(field, fmt.Errorf("must have at least one of: action, sh, windows"))
			continue
		}
		if hasAction && (hasSh || hasWindows) {
			errs = errs.Append(field, fmt.Errorf("action is mutually exclusive with sh and windows"))
			continue
		}
		if hasAction && !isValidAction(cmd.Action) {
			errs = errs.Append(field, fmt.Errorf("invalid action %q", cmd.Action))
		}

		// Validate windows entries
		for i, w := range cmd.Windows {
			wfield := fmt.Sprintf("%s.windows[%d]", field, i)
			if w.Name == "" {
				errs = errs.Append(wfield, fmt.Errorf("name is required"))
			}
		}

		// options.remote requires options.session_name
		if cmd.Options.Remote != "" && cmd.Options.SessionName == "" {
			errs = errs.Append(field+".options.remote", fmt.Errorf("remote requires session_name to be set"))
		}
		// options.background only meaningful when windows are present
		if cmd.Options.Background && !hasWindows {
			errs = errs.Append(field+".options.background", fmt.Errorf("background only applies when windows are defined"))
		}
		// options.session_name only applies when windows are defined
		if cmd.Options.SessionName != "" && !hasWindows {
			errs = errs.Append(field+".options.session_name", fmt.Errorf("session_name only applies when windows are defined"))
		}

		// Validate scope values
		for _, scope := range cmd.Scope {
			if !isValidScope(scope) {
				errs = errs.Append(field+".scope", fmt.Errorf("invalid scope %q: must be one of: global, sessions, messages, review", scope))
			}
		}

		// Validate form fields
		if len(cmd.Form) > 0 {
			if cmd.Action != "" {
				errs = errs.Append(field, fmt.Errorf("form can only be used with sh, not action"))
				continue
			}

			if err := criterio.ValidateSlice(field+".form", cmd.Form, validateFormField); err != nil {
				errs = errs.Append("", err)
				continue
			}

			// Check for duplicate variables
			seenVars := make(map[string]bool, len(cmd.Form))
			for j, ff := range cmd.Form {
				if seenVars[ff.Variable] {
					errs = errs.Append(fmt.Sprintf("%s.form[%d].variable", field, j),
						fmt.Errorf("duplicate variable %q", ff.Variable))
				}
				seenVars[ff.Variable] = true
			}
		}
	}

	return errs.ToError()
}

// validateFormField validates a single form field definition.
func validateFormField(ff FormField) error {
	var extra criterio.FieldErrorsBuilder

	// Cross-field: exactly one of type or preset
	switch {
	case ff.Type == "" && ff.Preset == "":
		extra = extra.Append("type", fmt.Errorf("must specify either type or preset"))
	case ff.Type != "" && ff.Preset != "":
		extra = extra.Append("type", fmt.Errorf("cannot specify both type and preset"))
	}

	// Select types require options (when not a preset)
	if (ff.Type == FormTypeSelect || ff.Type == FormTypeMultiSelect) && len(ff.Options) == 0 {
		extra = extra.Append("options", fmt.Errorf("required for %s type", ff.Type))
	}

	// Filter is only valid for SessionSelector preset
	if ff.Filter != "" && ff.Preset != FormPresetSessionSelector {
		extra = extra.Append("filter", fmt.Errorf("only valid for %s preset", FormPresetSessionSelector))
	}

	return criterio.ValidateStruct(
		criterio.Run("variable", ff.Variable, criterio.Required[string], isValidIdentifier),
		criterio.Run("type", ff.Type,
			criterio.When(ff.Type != "", criterio.StrOneOf(ValidFormTypes...)),
		),
		criterio.Run("preset", ff.Preset,
			criterio.When(ff.Preset != "", criterio.StrOneOf(ValidFormPresets...)),
		),
		criterio.Run("filter", ff.Filter,
			criterio.When(ff.Filter != "", criterio.StrOneOf(ValidSessionFilters...)),
		),
		extra.ToError(),
	)
}

// isValidIdentifier checks that a string is a valid Go-style identifier
// (letters, digits, underscores; must not start with a digit).
var isValidIdentifier criterio.Validator[string] = func(s string) error {
	for i, r := range s {
		if i == 0 && r >= '0' && r <= '9' {
			return fmt.Errorf("must not start with a digit")
		}
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
			return fmt.Errorf("must contain only letters, digits, and underscores")
		}
	}
	return nil
}

// validateWindowsBasic checks windows config for structural validity.
func (c *Config) validateWindowsBasic() error {
	var errs criterio.FieldErrorsBuilder
	for i, rule := range c.Rules {
		if len(rule.Windows) == 0 {
			continue
		}

		// Windows and spawn/batch_spawn are mutually exclusive
		if len(rule.Spawn) > 0 {
			errs = errs.Append(fmt.Sprintf("rules[%d]", i), fmt.Errorf("cannot have both windows and spawn"))
		}
		if len(rule.BatchSpawn) > 0 {
			errs = errs.Append(fmt.Sprintf("rules[%d]", i), fmt.Errorf("cannot have both windows and batch_spawn"))
		}

		// Each window must have a name
		for j, w := range rule.Windows {
			if w.Name == "" {
				errs = errs.Append(fmt.Sprintf("rules[%d].windows[%d].name", i, j), fmt.Errorf("is required"))
			}
		}
	}
	return errs.ToError()
}

// validateMaxRecycled checks that max_recycled values are non-negative.
func (c *Config) validateMaxRecycled() error {
	var errs criterio.FieldErrorsBuilder

	for i, rule := range c.Rules {
		if rule.MaxRecycled != nil && *rule.MaxRecycled < 0 {
			errs = errs.Append(fmt.Sprintf("rules[%d].max_recycled", i), fmt.Errorf("must be >= 0, got %d", *rule.MaxRecycled))
		}
	}

	return errs.ToError()
}

// validateAgents checks that agents.default references an existing profile.
func (c *Config) validateAgents() error {
	if _, ok := c.Agents.Profiles[c.Agents.Default]; !ok {
		return criterio.NewFieldErrors("agents.default", fmt.Errorf("profile %q not found in agents config", c.Agents.Default))
	}
	return nil
}

// validateTheme checks that the configured theme name is a valid built-in theme.
func (c *Config) validateTheme() error {
	if _, ok := styles.GetPalette(c.TUI.Theme); !ok {
		return fmt.Errorf("tui.theme: unknown theme %q, available themes: %v", c.TUI.Theme, styles.ThemeNames())
	}
	return nil
}

// validateKeybindingsBasic performs basic keybinding validation for the Validate() method.
// Only checks structural validity (non-empty cmd). Command reference validation
// is done earlier via validateUserKeybindings() before defaults are merged,
// since default keybindings may reference plugin commands not yet registered.
func (c *Config) validateKeybindingsBasic() error {
	var errs criterio.FieldErrorsBuilder

	for key, kb := range c.Keybindings {
		field := fmt.Sprintf("keybindings[%q]", key)

		if kb.Cmd == "" {
			errs = errs.Append(field, fmt.Errorf("cmd is required"))
		}
	}

	return errs.ToError()
}

// validateUserKeybindings validates user-defined keybindings reference valid commands.
// Called before merging with defaults so only user-provided keybindings are checked.
func (c *Config) validateUserKeybindings() error {
	var errs criterio.FieldErrorsBuilder

	allCommands := c.MergedUserCommands()

	// Commands referenced by default keybindings are valid targets for user rebinding.
	// This includes plugin commands (TmuxOpen, etc.) not yet registered at config-load time.
	defaultBindingCmds := make(map[string]bool, len(defaultKeybindings))
	for _, kb := range defaultKeybindings {
		defaultBindingCmds[kb.Cmd] = true
	}

	for key, kb := range c.Keybindings {
		field := fmt.Sprintf("keybindings[%q]", key)

		if kb.Cmd == "" {
			errs = errs.Append(field, fmt.Errorf("cmd is required"))
			continue
		}

		if _, exists := allCommands[kb.Cmd]; !exists && !defaultBindingCmds[kb.Cmd] {
			errs = errs.Append(field, fmt.Errorf("cmd %q does not reference a valid user command", kb.Cmd))
		}
	}

	return errs.ToError()
}

// ReposDir returns the path where cloned repositories are stored.
func (c *Config) ReposDir() string {
	return filepath.Join(c.DataDir, "repos")
}

// SessionsFile returns the path to the sessions JSON file.
func (c *Config) SessionsFile() string {
	return filepath.Join(c.DataDir, "sessions.json")
}

// HistoryFile returns the path to the command history JSON file.
func (c *Config) HistoryFile() string {
	return filepath.Join(c.DataDir, "history.json")
}

// LogsDir returns the path to the logs directory.
func (c *Config) LogsDir() string {
	return filepath.Join(c.DataDir, "logs")
}

// ContextDir returns the base context directory path.
func (c *Config) ContextDir() string {
	return filepath.Join(c.DataDir, "context")
}

// RepoContextDir returns the context directory for a specific owner/repo.
func (c *Config) RepoContextDir(owner, repo string) string {
	return filepath.Join(c.ContextDir(), owner, repo)
}

// SharedContextDir returns the shared context directory.
func (c *Config) SharedContextDir() string {
	return filepath.Join(c.ContextDir(), "shared")
}

// DatabaseFile returns the path to the SQLite database file.
func (c *Config) DatabaseFile() string {
	return filepath.Join(c.DataDir, "hive.db")
}

// BinDir returns the path to the extracted bundled scripts directory.
func (c *Config) BinDir() string {
	return filepath.Join(c.DataDir, "bin")
}

func isValidAction(t action.Type) bool {
	return t.IsValid() && action.IsConfigAction(t)
}

// isValidScope checks if a scope value is valid.
func isValidScope(scope string) bool {
	switch scope {
	case "global", "sessions", "messages", "review":
		return true
	default:
		return false
	}
}

// isValidCommandName checks if a command name is valid.
// Valid names contain only alphanumeric characters, dashes, and underscores.
func isValidCommandName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

// DefaultMaxRecycled is the default limit for recycled sessions per repository.
const DefaultMaxRecycled = 5

// GetMaxRecycled returns the max recycled sessions limit for the given remote URL.
// Returns DefaultMaxRecycled (5) if no limit is configured.
// Returns 0 for unlimited.
func (c *Config) GetMaxRecycled(remote string) int {
	// Check rules in order - last matching rule with MaxRecycled set wins
	var result *int
	for _, rule := range c.Rules {
		if rule.Pattern == "" || matchesPattern(rule.Pattern, remote) {
			if rule.MaxRecycled != nil {
				result = rule.MaxRecycled
			}
		}
	}

	if result != nil {
		return *result
	}

	return DefaultMaxRecycled
}

// SpawnStrategy holds the resolved spawn method for a session.
// Exactly one of Windows or Commands is populated.
type SpawnStrategy struct {
	Windows  []WindowConfig
	Commands []string
}

// IsWindows returns true if the strategy uses declarative window config.
func (s SpawnStrategy) IsWindows() bool { return len(s.Windows) > 0 }

// ResolveSpawn determines the spawn strategy for the given remote URL.
// Rules are evaluated in order (last-match-wins). If the last matching rule
// has windows, those are used. If it has spawn/batch_spawn commands, those are used.
// If nothing matches, DefaultWindows() is returned.
func ResolveSpawn(rules []Rule, remote string, batch bool) SpawnStrategy {
	var strategy SpawnStrategy
	for _, rule := range rules {
		if !rule.Matches(remote) {
			continue
		}
		switch {
		case len(rule.Windows) > 0:
			strategy = SpawnStrategy{Windows: rule.Windows}
		case batch && len(rule.BatchSpawn) > 0:
			strategy = SpawnStrategy{Commands: rule.BatchSpawn}
		case !batch && len(rule.Spawn) > 0:
			strategy = SpawnStrategy{Commands: rule.Spawn}
		}
	}
	if !strategy.IsWindows() && len(strategy.Commands) == 0 {
		return SpawnStrategy{Windows: DefaultWindows()}
	}
	return strategy
}

// GetRecycleCommands returns the recycle commands for the given remote URL.
// Rules are evaluated in order; the last matching rule with recycle commands wins.
// If no rules define recycle commands, returns DefaultRecycleCommands.
func (c *Config) GetRecycleCommands(remote string) []string {
	var result []string
	for _, rule := range c.Rules {
		if rule.Pattern == "" || matchesPattern(rule.Pattern, remote) {
			if len(rule.Recycle) > 0 {
				result = rule.Recycle
			}
		}
	}
	if len(result) == 0 {
		return DefaultRecycleCommands
	}
	return result
}

// Matches reports whether this rule matches the given remote URL.
// An empty pattern matches everything.
func (r Rule) Matches(remote string) bool {
	if r.Pattern == "" {
		return true
	}
	return matchesPattern(r.Pattern, remote)
}

// matchesPattern checks if remote matches the regex pattern.
func matchesPattern(pattern, remote string) bool {
	matched, _ := filepath.Match(pattern, remote)
	if matched {
		return true
	}
	// Try regex matching
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(remote)
}
