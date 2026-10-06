// Package config holds the hive CLI's configuration: the engine sections from
// internal/config plus the sections only the CLI reads (views, keybindings,
// user commands, TUI, plugins, sources). Both are read from the same
// config.yaml.
package config

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/colonyops/hive/cmd/hive/internal/action"
	"github.com/colonyops/hive/cmd/hive/internal/theme"
	"github.com/colonyops/hive/internal/config"
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
	Variable    string   `json:"variable"              yaml:"variable"`              // Template variable name (under .Form)
	Type        string   `json:"type,omitempty"        yaml:"type,omitempty"`        // text, textarea, select, multi-select
	Preset      string   `json:"preset,omitempty"      yaml:"preset,omitempty"`      // SessionSelector, ProjectSelector
	Label       string   `json:"label"                 yaml:"label"`                 // Display label
	Placeholder string   `json:"placeholder,omitempty" yaml:"placeholder,omitempty"` // Input placeholder text
	Default     string   `json:"default,omitempty"     yaml:"default,omitempty"`     // Default value (text/textarea/select)
	Options     []string `json:"options,omitempty"     yaml:"options,omitempty"`     // Static options for select/multi-select
	Multi       bool     `json:"multi,omitempty"       yaml:"multi,omitempty"`       // For presets: enable multi-select
	Filter      string   `json:"filter,omitempty"      yaml:"filter,omitempty"`      // For SessionSelector: "active" (default) or "all"
}

// defaultUserCommands provides built-in commands that users can override.
// Commands with nil Scope are available in all views (global visibility).
var defaultUserCommands = map[string]UserCommand{
	"Recycle": {
		Action:  action.TypeRecycle,
		Help:    "recycle",
		Confirm: "Are you sure you want to recycle this session?",
		Scope:   []string{"sessions"},
	},
	"Delete": {
		Action:  action.TypeDelete,
		Help:    "delete",
		Confirm: "Are you sure you want to delete this session?",
		Scope:   []string{"sessions"},
	},
	"NewSession": {
		Action: action.TypeNewSession,
		Help:   "new session",
		Silent: true,
		Scope:  []string{"sessions"},
	},
	"DocReview": {
		Action: action.TypeDocReview,
		Help:   "review documents",
		Silent: true,
		Scope:  []string{"sessions"},
	},
	"FilterAll": {
		Action: action.TypeFilterAll,
		Help:   "show all sessions",
		Silent: true,
		Scope:  []string{"sessions"},
	},
	"FilterActive": {
		Action: action.TypeFilterActive,
		Help:   "show sessions with active agents",
		Silent: true,
		Scope:  []string{"sessions"},
	},
	"FilterApproval": {
		Action: action.TypeFilterApproval,
		Help:   "show sessions needing approval",
		Silent: true,
		Scope:  []string{"sessions"},
	},
	"FilterReady": {
		Action: action.TypeFilterReady,
		Help:   "show sessions with idle agents",
		Silent: true,
		Scope:  []string{"sessions"},
	},
	"ThemePreview": {
		Action: action.TypeSetTheme,
		Help:   "preview theme (" + strings.Join(theme.Names(), ", ") + ")",
		Silent: true,
	},
	"Notifications": {
		Action: action.TypeNotifications,
		Help:   "show notification history",
		Silent: true,
	},
	"RenameSession": {
		Action: action.TypeRenameSession,
		Help:   "rename session",
		Silent: true,
		Scope:  []string{"sessions"},
	},
	"NextActive": {
		Action: action.TypeNextActive,
		Help:   "next active session",
		Silent: true,
		Scope:  []string{"sessions"},
	},
	"PrevActive": {
		Action: action.TypePrevActive,
		Help:   "prev active session",
		Silent: true,
		Scope:  []string{"sessions"},
	},
	"HiveInfo": {
		Action: action.TypeHiveInfo,
		Help:   "show build and config info",
		Silent: true,
	},
	"HiveDoctor": {
		Action: action.TypeHiveDoctor,
		Help:   "run health checks",
		Silent: true,
	},
	"GroupSet": {
		Action: action.TypeGroupSet,
		Help:   "set session group",
		Silent: true,
		Scope:  []string{"sessions"},
	},
	"GroupToggle": {
		Action: action.TypeGroupToggle,
		Help:   "toggle group/repo view",
		Silent: true,
		Scope:  []string{"sessions"},
	},
	"TodoPanel": {
		Action: action.TypeTodoPanel,
		Help:   "open todo panel",
		Silent: true,
		Scope:  []string{"sessions"},
	},
	"Sources": {
		Action: action.TypeOpenSourcePicker,
		Help:   "open a source picker (id/scope auto-detected when omittable; usage: Sources [id] [scope])",
		Silent: true,
		Scope:  []string{"sessions"},
	},
	"SourceIssues": {
		Action: action.TypeOpenSourcePicker,
		Args:   []string{"issues"},
		Help:   "browse GitHub issues and create a session (usage: SourceIssues [scope])",
		Silent: true,
		Scope:  []string{"sessions"},
	},
	"SourcePRs": {
		Action: action.TypeOpenSourcePicker,
		Args:   []string{"prs"},
		Help:   "browse GitHub pull requests and create a session (usage: SourcePRs [scope])",
		Silent: true,
		Scope:  []string{"sessions"},
	},
	"ViewTasks": {
		Action: action.TypeViewTasks,
		Help:   "view tasks for repo",
		Silent: true,
		Scope:  []string{"sessions"},
	},
	"TasksRefresh": {
		Action: action.TypeTasksRefresh,
		Help:   "reload tasks from store",
		Silent: true,
		Scope:  []string{"tasks"},
	},
	"TasksFilter": {
		Action: action.TypeTasksFilter,
		Help:   "cycle status filter (open/all/done)",
		Silent: true,
		Scope:  []string{"tasks"},
	},
	"TasksCopyID": {
		Action: action.TypeTasksCopyID,
		Help:   "copy item ID to clipboard",
		Silent: true,
		Scope:  []string{"tasks"},
	},
	"TasksTogglePreview": {
		Action: action.TypeTasksTogglePreview,
		Help:   "show or hide detail panel",
		Silent: true,
		Scope:  []string{"tasks"},
	},
	"TasksSelectRepo": {
		Action: action.TypeTasksSelectRepo,
		Help:   "switch repository scope",
		Silent: true,
		Scope:  []string{"tasks"},
	},
	"TasksSetOpen": {
		Action: action.TypeTasksSetOpen,
		Help:   "mark task as open",
		Silent: true,
		Scope:  []string{"tasks"},
	},
	"TasksSetInProgress": {
		Action: action.TypeTasksSetInProgress,
		Help:   "mark task as in progress",
		Silent: true,
		Scope:  []string{"tasks"},
	},
	"TasksSetDone": {
		Action: action.TypeTasksSetDone,
		Help:   "mark task as done",
		Silent: true,
		Scope:  []string{"tasks"},
	},
	"TasksSetCancelled": {
		Action: action.TypeTasksSetCancelled,
		Help:   "mark task as cancelled",
		Silent: true,
		Scope:  []string{"tasks"},
	},
	"TasksDelete": {
		Action: action.TypeTasksDelete,
		Help:   "delete selected item",
		Silent: true,
		Scope:  []string{"tasks"},
	},
	"TasksPrune": {
		Action: action.TypeTasksPrune,
		Help:   "remove completed items",
		Silent: true,
		Scope:  []string{"tasks"},
	},
	"DocsCopyPath": {
		Action: action.TypeDocsCopyPath,
		Help:   "copy path",
		Silent: true,
		Scope:  []string{"review"},
	},
	"DocsCopyRelPath": {
		Action: action.TypeDocsCopyRelPath,
		Help:   "copy relative path",
		Silent: true,
		Scope:  []string{"review"},
	},
	"DocsCopyContents": {
		Action: action.TypeDocsCopyContents,
		Help:   "copy contents",
		Silent: true,
		Scope:  []string{"review"},
	},
	"DocsOpen": {
		Action: action.TypeDocsOpen,
		Help:   "open in editor",
		Silent: true,
		Scope:  []string{"review"},
	},
	"DocsTogglePreview": {
		Action: action.TypeDocsTogglePreview,
		Help:   "show or hide detail panel",
		Silent: true,
		Scope:  []string{"review"},
	},
	"DocsToggleTree": {
		Action: action.TypeDocsToggleTree,
		Help:   "show or hide folder tree",
		Silent: true,
		Scope:  []string{"review"},
	},
	"DocsSelectRepo": {
		Action: action.TypeDocsSelectRepo,
		Help:   "switch repository",
		Silent: true,
		Scope:  []string{"review"},
	},
	"SessionsRefreshGitStatuses": {
		Action: action.TypeSessionsRefreshGitStatuses,
		Help:   "refresh git status",
		Silent: true,
		Scope:  []string{"sessions"},
	},
	"WorkspaceRefresh": {
		Action: action.TypeWorkspaceRefresh,
		Help:   "refresh workspace repositories",
		Silent: true,
		Scope:  []string{"sessions"},
	},
	"SessionsTogglePreview": {
		Action: action.TypeSessionsTogglePreview,
		Help:   "toggle preview pane",
		Silent: true,
		Scope:  []string{"sessions"},
	},
	"SessionsNavigateUp": {
		Action: action.TypeSessionsNavigateUp,
		Help:   "move selection up",
		Silent: true,
		Scope:  []string{"sessions"},
	},
	"SessionsNavigateDown": {
		Action: action.TypeSessionsNavigateDown,
		Help:   "move selection down",
		Silent: true,
		Scope:  []string{"sessions"},
	},
	"SessionsFilterStart": {
		Action: action.TypeSessionsFilterStart,
		Help:   "start filter",
		Silent: true,
		Scope:  []string{"sessions"},
	},
	"SessionsCommandPaletteOpen": {
		Action: action.TypeSessionsCommandPaletteOpen,
		Help:   "open command palette",
		Silent: true,
		Scope:  []string{"sessions"},
	},
	"GoToTop": {
		Action: action.TypeGoToTop,
		Help:   "jump to top",
		Silent: true,
		Scope:  []string{"tasks", "review"},
	},
	"GoToBottom": {
		Action: action.TypeGoToBottom,
		Help:   "jump to bottom",
		Silent: true,
		Scope:  []string{"tasks", "review"},
	},
	"Quit": {
		Action: action.TypeQuit,
		Help:   "quit",
		Silent: true,
	},
	"ShowHelp": {
		Action: action.TypeShowHelp,
		Help:   "show help",
		Silent: true,
	},
	"SendBatch": {
		Sh: `{{ range .Form.targets }}
{{ agentSend }} {{ printf "=%s" .Name | shq }}:claude {{ $.Form.message | shq }}
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
		Scope:  []string{"sessions"},
	},
}

// CurrentConfigVersion is the latest config schema version.
// Increment this when making breaking changes to config format.
const CurrentConfigVersion = "0.2.7"

// Config is the CLI's configuration. The engine sections are inlined, so YAML
// keys are the same as in a file only the engine reads.
type Config struct {
	config.Config `yaml:",inline"`

	Version      string                 `json:"version"      yaml:"version"`
	CopyCommand  string                 `json:"copy_command" yaml:"copy_command"` // command to copy to clipboard (e.g., pbcopy, xclip)
	Keybindings  map[string]Keybinding  `json:"keybindings"  yaml:"keybindings"`
	UserCommands map[string]UserCommand `json:"usercommands" yaml:"usercommands"`
	History      HistoryConfig          `json:"history"      yaml:"history"`
	TUI          TUIConfig              `json:"tui"          yaml:"tui"`
	Review       ReviewConfig           `json:"review"       yaml:"review"`
	Plugins      PluginsConfig          `json:"plugins"      yaml:"plugins"`
	Sources      SourcesConfig          `json:"sources"      yaml:"sources"`
	Views        ViewsConfig            `json:"views"        yaml:"views"`

	hasLegacyKeybindings bool `json:"-" yaml:"-"`
}

// HistoryConfig holds command history configuration.
type HistoryConfig struct {
	MaxEntries int `json:"max_entries" yaml:"max_entries"`
}

// Group-by mode constants for tree view grouping.
const (
	GroupByRepo  = "repo"  // Group sessions by repository (default)
	GroupByGroup = "group" // Group sessions by user-assigned group
)

// ValidGroupByModes lists all valid group_by values.
var ValidGroupByModes = []string{GroupByRepo, GroupByGroup}

// TUIConfig holds TUI-related configuration.
type TUIConfig struct {
	Theme         string `json:"theme"          yaml:"theme"`          // built-in theme name (default: "tokyo-night")
	Icons         *bool  `json:"icons"          yaml:"icons"`          // enable nerd font icons (nil = true by default)
	UpdateChecker bool   `json:"update_checker" yaml:"update_checker"` // enable startup update checker (default: true)
	Store         bool   `json:"store"          yaml:"store"`          // KV store browser (default: false)
}

// ReviewConfig holds review-related configuration.
type ReviewConfig struct{}

// IconsEnabled returns true if nerd font icons should be shown.
func (t TUIConfig) IconsEnabled() bool {
	return t.Icons == nil || *t.Icons
}

// PluginsConfig holds configuration for the plugin system.
type PluginsConfig struct {
	ShellWorkers int                    `json:"shell_workers" yaml:"shell_workers"` // shared subprocess pool size (default: 5)
	GitHub       GitHubPluginConfig     `json:"github"        yaml:"github"`
	LazyGit      LazyGitPluginConfig    `json:"lazygit"       yaml:"lazygit"`
	Neovim       NeovimPluginConfig     `json:"neovim"        yaml:"neovim"`
	ContextDir   ContextDirPluginConfig `json:"contextdir"    yaml:"contextdir"`
	Claude       ClaudePluginConfig     `json:"claude"        yaml:"claude"`
	Tmux         TmuxPluginConfig       `json:"tmux"          yaml:"tmux"`
}

// TmuxPluginConfig holds tmux plugin configuration.
type TmuxPluginConfig struct {
	Enabled *bool `json:"enabled" yaml:"enabled"` // nil = auto-detect, true/false = override
}

// GitHubPluginConfig holds GitHub plugin configuration.
type GitHubPluginConfig struct {
	Enabled      *bool         `json:"enabled"       yaml:"enabled"`       // nil = auto-detect, true/false = override
	ResultsCache time.Duration `json:"results_cache" yaml:"results_cache"` // status cache duration (default: 8m)
}

// LazyGitPluginConfig holds lazygit plugin configuration.
type LazyGitPluginConfig struct {
	Enabled *bool `json:"enabled" yaml:"enabled"` // nil = auto-detect, true/false = override
}

// NeovimPluginConfig holds neovim plugin configuration.
type NeovimPluginConfig struct {
	Enabled *bool `json:"enabled" yaml:"enabled"` // nil = auto-detect, true/false = override
}

// ContextDirPluginConfig holds context directory plugin configuration.
type ContextDirPluginConfig struct {
	Enabled *bool `json:"enabled" yaml:"enabled"` // nil = auto-detect, true/false = override
}

// ClaudePluginConfig holds Claude Code plugin configuration.
type ClaudePluginConfig struct {
	Enabled *bool `json:"enabled" yaml:"enabled"` // nil = auto-detect, true/false = override
}

// SourcesConfig configures the sources system (GitHub issues and pull
// requests browsable from the picker).
type SourcesConfig struct {
	SearchLimit int                 `json:"search_limit" yaml:"search_limit"` // max items per search (default: 30)
	CacheTTL    time.Duration       `json:"cache_ttl"    yaml:"cache_ttl"`    // search result cache TTL (default: 30s)
	Hosts       map[string]string   `json:"hosts"        yaml:"hosts"`        // git remote host -> backend ("github"|"gitea"); overrides auto-detection
	Issues      BuiltinSourceConfig `json:"issues"       yaml:"issues"`
	PRs         BuiltinSourceConfig `json:"prs"          yaml:"prs"`
}

// SourceTemplateConfig holds the templates rendering a selected item
// into session Name/Prompt/Tags.
type SourceTemplateConfig struct {
	Name   string   `json:"name"   yaml:"name"`
	Prompt string   `json:"prompt" yaml:"prompt"`
	Tags   []string `json:"tags"   yaml:"tags"`
}

// BuiltinSourceConfig configures one built-in source (issues, prs).
type BuiltinSourceConfig struct {
	Enabled   *bool                `json:"enabled"   yaml:"enabled"` // nil = auto-detect, true/false = override
	Templates SourceTemplateConfig `json:"templates" yaml:"templates"`
}

// Keybinding defines a TUI keybinding that references a UserCommand.
type Keybinding struct {
	Cmd     string `json:"cmd"     yaml:"cmd"`     // command name (required, references UserCommand)
	Help    string `json:"help"    yaml:"help"`    // optional override for help text
	Confirm string `json:"confirm" yaml:"confirm"` // optional override for confirmation prompt
}

// UserCommandOptions controls execution behaviour for UserCommands with windows.
type UserCommandOptions struct {
	// SessionName is a template string. When non-empty, a new Hive session is created
	// with this name before sh: runs and windows are opened.
	SessionName string `json:"session_name,omitempty" yaml:"session_name,omitempty"`
	// Remote overrides the remote URL for new session creation.
	// Only valid when SessionName is also set.
	Remote string `json:"remote,omitempty" yaml:"remote,omitempty"`
	// Background creates windows without attaching or switching to the tmux session.
	Background bool `json:"background,omitempty" yaml:"background,omitempty"`
}

// UserCommand defines a named command accessible via command palette or keybindings.
type UserCommand struct {
	Action  action.Type           `json:"action,omitempty"  yaml:"action,omitempty"`  // built-in action (Recycle, Delete, etc.) - mutually exclusive with sh
	Args    []string              `json:"args,omitempty"    yaml:"args,omitempty"`    // preset args for action commands; typed palette args are appended after these
	Sh      string                `json:"sh"                yaml:"sh"`                // shell command template - mutually exclusive with action
	Windows []config.WindowConfig `json:"windows,omitempty" yaml:"windows,omitempty"` // Tmux windows to open after sh: completes
	Options UserCommandOptions    `json:"options,omitempty" yaml:"options,omitempty"` // Execution options for window-based commands
	Form    []FormField           `json:"form,omitempty"    yaml:"form,omitempty"`    // interactive input fields collected before sh execution
	Help    string                `json:"help"              yaml:"help"`              // description shown in palette/help
	Confirm string                `json:"confirm"           yaml:"confirm"`           // confirmation prompt (empty = no confirm)
	Silent  bool                  `json:"silent"            yaml:"silent"`            // skip loading popup for fast commands
	Exit    string                `json:"exit"              yaml:"exit"`              // exit hive after command (bool or $ENV_VAR)
	Scope   []string              `json:"scope,omitempty"   yaml:"scope,omitempty"`   // views where command is active (empty = global)
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

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Config:      config.DefaultConfig(),
		Keybindings: map[string]Keybinding{},
		History: HistoryConfig{
			MaxEntries: 100,
		},
		TUI: TUIConfig{
			UpdateChecker: true,
		},
		Views: ViewsConfig{
			Sessions: SessionsViewConfig{
				RefreshInterval: 15 * time.Second,
				PreviewEnabled:  true,
			},
		},
		Sources: SourcesConfig{
			Issues: BuiltinSourceConfig{
				Templates: SourceTemplateConfig{
					Name: "gh-{{ .Fields.number }}-{{ .Title }}",
					// .Detail is the issue body, fetched at selection time
					// in both the CLI and TUI paths. The CLI fails hard on
					// fetch errors; the TUI is best-effort and falls back to
					// an empty detail. Rendered prompts are trimmed, so an
					// empty detail leaves no dangling blank lines.
					Prompt: "Work on {{ .Title }}\n\n{{ .Fields.url }}\n\n{{ .Detail }}",
					Tags:   []string{"github", "issue-{{ .Fields.number }}"},
				},
			},
			PRs: BuiltinSourceConfig{
				Templates: SourceTemplateConfig{
					Name:   "gh-pr-{{ .Fields.number }}-{{ .Title }}",
					Prompt: "Review pull request {{ .Title }}\n\n{{ .Fields.url }}",
					Tags:   []string{"github", "pr-{{ .Fields.number }}"},
				},
			},
		},
	}
}

// Load reads configuration from the given path and sets the data directory.
// If configPath is empty or doesn't exist, returns defaults with the provided dataDir.
func Load(configPath, dataDir string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := config.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	if data != nil {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parse config file: %w", err)
		}
	}

	// Validate user keybindings before merging defaults (defaults may reference
	// plugin commands that don't exist at config-load time).
	if err := cfg.validateUserKeybindings(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// The top-level keybindings field is deprecated in favor of views.sessions.keybindings.
	// Migrate any entries the user still has in the old location.
	if len(cfg.Keybindings) > 0 {
		cfg.hasLegacyKeybindings = true
		if cfg.Views.Sessions.Keybindings == nil {
			cfg.Views.Sessions.Keybindings = make(map[string]Keybinding)
		}
		maps.Copy(cfg.Views.Sessions.Keybindings, cfg.Keybindings)
	}

	// Merge per-view defaults (defaults first, user config overrides)
	cfg.Views = mergeViewsConfig(defaultViewsConfig, cfg.Views)

	// Rebuild flat keybindings map (global + sessions merged) so code that
	// reads cfg.Keybindings directly still works. Note this includes global
	// keybindings merged in, unlike the old raw-user-input semantics.
	cfg.Keybindings = cfg.Views.flattenedForView("sessions")

	cfg.Complete(dataDir)
	cfg.applyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// applyDefaults sets default values for unset CLI options. The engine's
// defaults are applied by config.Config.Complete.
func (c *Config) applyDefaults() {
	defaults := DefaultConfig()
	if c.History.MaxEntries == 0 {
		c.History.MaxEntries = defaults.History.MaxEntries
	}
	if c.TUI.Theme == "" {
		c.TUI.Theme = theme.Default
	}
	if c.Views.Sessions.GroupBy == "" {
		c.Views.Sessions.GroupBy = GroupByRepo
	}
	if c.CopyCommand == "" {
		c.CopyCommand = defaultCopyCommand()
	}
	if c.Plugins.ShellWorkers == 0 {
		c.Plugins.ShellWorkers = 5
	}
	if c.Plugins.GitHub.ResultsCache == 0 {
		c.Plugins.GitHub.ResultsCache = 8 * time.Minute
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

// Validate checks the engine sections and the CLI sections.
func (c *Config) Validate() error {
	return criterio.ValidateStruct(
		c.Config.Validate(),
		c.validateTheme(),
		c.validateGroupBy(),
		c.validateKeybindingsBasic(),
		c.validateUserCommandsBasic(),
		c.validateSources(),
	)
}

// validateUserCommandsBasic performs basic usercommand validation for the Validate() method.
func (c *Config) validateUserCommandsBasic() error {
	var errs criterio.FieldErrorsBuilder
	for name, cmd := range c.UserCommands {
		field := fmt.Sprintf("usercommands[%q]", name)
		errs = append(errs, ValidateUserCommandBasic(field, name, cmd)...)
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

// validateTheme checks that the configured theme name is a valid built-in theme.
func (c *Config) validateTheme() error {
	if _, ok := theme.Get(c.TUI.Theme); !ok {
		return fmt.Errorf("tui.theme: unknown theme %q, available themes: %v", c.TUI.Theme, theme.Names())
	}
	return nil
}

// validateGroupBy checks that the configured group_by value is valid.
func (c *Config) validateGroupBy() error {
	return criterio.Run("views.sessions.group_by", c.Views.Sessions.GroupBy, criterio.StrOneOf(ValidGroupByModes...))
}

// validateKeybindingsBasic performs basic keybinding validation for the Validate() method.
// Only checks structural validity (non-empty cmd). Command reference validation
// is done earlier via validateUserKeybindings() before defaults are merged,
// since default keybindings may reference plugin commands not yet registered.
func (c *Config) validateKeybindingsBasic() error {
	var errs criterio.FieldErrorsBuilder
	validateKeybindingMap(&errs, "keybindings", c.Keybindings)
	validateViewKeybindingMaps(&errs, c.Views)
	return errs.ToError()
}

// validateUserKeybindings validates user-defined keybindings before merging with defaults.
// Only structural validity (non-empty cmd) is checked here because plugin commands are
// registered at runtime and are not known at config-load time. Command existence is
// validated at the point of invocation.
func (c *Config) validateUserKeybindings() error {
	var errs criterio.FieldErrorsBuilder
	validateKeybindingMap(&errs, "keybindings", c.Keybindings)
	validateViewKeybindingMaps(&errs, c.Views)
	return errs.ToError()
}

func validateKeybindingMap(errs *criterio.FieldErrorsBuilder, prefix string, kbs map[string]Keybinding) {
	for key, kb := range kbs {
		if kb.Cmd == "" {
			*errs = errs.Append(fmt.Sprintf("%s[%q]", prefix, key), fmt.Errorf("cmd is required"))
		}
	}
}

// HistoryFile returns the path to the command history JSON file.
func (c *Config) HistoryFile() string {
	return filepath.Join(c.DataDir, "history.json")
}

func isValidAction(t action.Type) bool {
	return t.IsValid() && action.IsConfigAction(t)
}

// ValidScopes lists all valid scope values for user commands.
// "global" means the command is available everywhere; other values
// match ViewType.String() in the TUI package.
var ValidScopes = []string{"global", "sessions", "messages", "review", "todos", "tasks"}

// isValidScope checks if a scope value is valid.
func isValidScope(scope string) bool {
	for _, s := range ValidScopes {
		if s == scope {
			return true
		}
	}
	return false
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
