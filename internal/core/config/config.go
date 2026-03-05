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

// Clone strategy constants.
const (
	CloneStrategyFull     = "full"
	CloneStrategyWorktree = "worktree"
)

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
		Help:   "preview theme (" + strings.Join(styles.ThemeNames(), ", ") + ")",
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
		Scope:  []string{"sessions"},
	},
}

// CurrentConfigVersion is the latest config schema version.
// Increment this when making breaking changes to config format.
const CurrentConfigVersion = "0.2.7"

// Config holds the application configuration.
type Config struct {
	Version             string                 `json:"version"               yaml:"version"`
	CopyCommand         string                 `json:"copy_command"          yaml:"copy_command"` // command to copy to clipboard (e.g., pbcopy, xclip)
	Git                 GitConfig              `json:"git"                   yaml:"git"`
	GitPath             string                 `json:"git_path"              yaml:"git_path"`
	Keybindings         map[string]Keybinding  `json:"keybindings"           yaml:"keybindings"`
	UserCommands        map[string]UserCommand `json:"usercommands"          yaml:"usercommands"`
	Rules               []Rule                 `json:"rules"                 yaml:"rules"`
	Agents              AgentsConfig           `json:"agents"                yaml:"agents"`
	AutoDeleteCorrupted bool                   `json:"auto_delete_corrupted" yaml:"auto_delete_corrupted"`
	History             HistoryConfig          `json:"history"               yaml:"history"`
	Context             ContextConfig          `json:"context"               yaml:"context"`
	TUI                 TUIConfig              `json:"tui"                   yaml:"tui"`
	Review              ReviewConfig           `json:"review"                yaml:"review"`
	Messaging           MessagingConfig        `json:"messaging"             yaml:"messaging"`
	Tmux                TmuxConfig             `json:"tmux"                  yaml:"tmux"`
	Database            DatabaseConfig         `json:"database"              yaml:"database"`
	Plugins             PluginsConfig          `json:"plugins"               yaml:"plugins"`
	Todos               TodosConfig            `json:"todos"                 yaml:"todos"`
	Workspaces          []string               `json:"workspaces"            yaml:"workspaces"` // directories containing git repositories for new session dialog
	Views               ViewsConfig            `json:"views"                 yaml:"views"`
	RepoDirsCompat      []string               `json:"-"                     yaml:"repo_dirs"` // deprecated: use workspaces instead (kept for backwards compatibility)
	DataDir             string                 `json:"-"                     yaml:"-"`         // set by caller, not from config file

	hasLegacyKeybindings bool `json:"-" yaml:"-"`
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
	Command string   `json:"command" yaml:"command"` // CLI binary (defaults to profile key if omitted)
	Flags   []string `json:"flags"   yaml:"flags"`   // extra CLI args appended to command on spawn
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
	MaxEntries int `json:"max_entries" yaml:"max_entries"`
}

// ContextConfig configures context directory behavior.
type ContextConfig struct {
	SymlinkName string `json:"symlink_name" yaml:"symlink_name"` // default: ".hive"
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

// MessagingConfig holds messaging-related configuration.
type MessagingConfig struct {
	TopicPrefix string `json:"topic_prefix" yaml:"topic_prefix"` // default: "agent"
	MaxMessages int    `json:"max_messages" yaml:"max_messages"` // max messages per topic (default: 100, 0 = unlimited)
}

// TmuxConfig holds tmux integration configuration.
type TmuxConfig struct {
	PollInterval         time.Duration `json:"poll_interval"          yaml:"poll_interval"`          // status check frequency, default 1.5s
	PreviewWindowMatcher []string      `json:"preview_window_matcher" yaml:"preview_window_matcher"` // regex patterns for preferred window names (e.g., ["claude", "aider"])
}

// PluginsConfig holds configuration for the plugin system.
type PluginsConfig struct {
	ShellWorkers int                    `json:"shell_workers" yaml:"shell_workers"` // shared subprocess pool size (default: 5)
	GitHub       GitHubPluginConfig     `json:"github"        yaml:"github"`
	Beads        BeadsPluginConfig      `json:"beads"         yaml:"beads"`
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

// BeadsPluginConfig holds Beads plugin configuration.
type BeadsPluginConfig struct {
	Enabled      *bool         `json:"enabled"       yaml:"enabled"`       // nil = auto-detect, true/false = override
	ResultsCache time.Duration `json:"results_cache" yaml:"results_cache"` // status cache duration (default: 30s)
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
	Enabled         *bool         `json:"enabled"          yaml:"enabled"`          // nil = auto-detect, true/false = override
	CacheTTL        time.Duration `json:"cache_ttl"        yaml:"cache_ttl"`        // status cache duration (default: 30s)
	YellowThreshold int           `json:"yellow_threshold" yaml:"yellow_threshold"` // yellow above this % (default: 60)
	RedThreshold    int           `json:"red_threshold"    yaml:"red_threshold"`    // red above this % (default: 80)
	ModelLimit      int           `json:"model_limit"      yaml:"model_limit"`      // context limit (default: 200000)
}

// DatabaseConfig holds SQLite database configuration.
type DatabaseConfig struct {
	MaxOpenConns int `json:"max_open_conns" yaml:"max_open_conns"` // max open connections (default: 2)
	MaxIdleConns int `json:"max_idle_conns" yaml:"max_idle_conns"` // max idle connections (default: 2)
	BusyTimeout  int `json:"busy_timeout"   yaml:"busy_timeout"`   // busy timeout in milliseconds (default: 5000)
}

// GitConfig holds git-related configuration.
type GitConfig struct {
	StatusWorkers int `json:"status_workers" yaml:"status_workers"`
}

// WindowConfig defines a tmux window to create when spawning a session.
type WindowConfig struct {
	Name    string `json:"name"              yaml:"name"`              // Window name (template string, required)
	Command string `json:"command,omitempty" yaml:"command,omitempty"` // Command to run (template string, empty = shell)
	Dir     string `json:"dir,omitempty"     yaml:"dir,omitempty"`     // Working directory override (template string)
	Focus   bool   `json:"focus,omitempty"   yaml:"focus,omitempty"`   // Select this window after creation
}

// Rule defines actions to take for matching repositories.
type Rule struct {
	// Pattern matches against remote URL (regex). Empty = matches all.
	Pattern string `json:"pattern" yaml:"pattern"`
	// Commands to run in the session directory after clone/recycle.
	Commands []string `json:"commands,omitempty" yaml:"commands,omitempty"`
	// Copy are glob patterns to copy from source directory.
	Copy []string `json:"copy,omitempty" yaml:"copy,omitempty"`
	// MaxRecycled sets the max recycled sessions for matching repos.
	// nil = inherit from previous rule or default (5), 0 = unlimited, >0 = limit
	MaxRecycled *int `json:"max_recycled,omitempty" yaml:"max_recycled,omitempty"`
	// Windows defines tmux windows to create when spawning a session.
	// Mutually exclusive with Spawn/BatchSpawn.
	Windows []WindowConfig `json:"windows,omitempty" yaml:"windows,omitempty"`
	// Spawn commands to run when creating a new session (hive new).
	Spawn []string `json:"spawn,omitempty" yaml:"spawn,omitempty"`
	// BatchSpawn commands to run when creating a batch session (hive batch).
	BatchSpawn []string `json:"batch_spawn,omitempty" yaml:"batch_spawn,omitempty"`
	// Recycle commands to run when recycling a session.
	Recycle []string `json:"recycle,omitempty" yaml:"recycle,omitempty"`
	// CloneStrategy overrides the clone strategy for matching repos ("full" or "worktree").
	CloneStrategy string `json:"clone_strategy,omitempty" yaml:"clone_strategy,omitempty"`
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
	Action  action.Type        `json:"action,omitempty"  yaml:"action,omitempty"`  // built-in action (Recycle, Delete, etc.) - mutually exclusive with sh
	Sh      string             `json:"sh"                yaml:"sh"`                // shell command template - mutually exclusive with action
	Windows []WindowConfig     `json:"windows,omitempty" yaml:"windows,omitempty"` // Tmux windows to open after sh: completes
	Options UserCommandOptions `json:"options,omitempty" yaml:"options,omitempty"` // Execution options for window-based commands
	Form    []FormField        `json:"form,omitempty"    yaml:"form,omitempty"`    // interactive input fields collected before sh execution
	Help    string             `json:"help"              yaml:"help"`              // description shown in palette/help
	Confirm string             `json:"confirm"           yaml:"confirm"`           // confirmation prompt (empty = no confirm)
	Silent  bool               `json:"silent"            yaml:"silent"`            // skip loading popup for fast commands
	Exit    string             `json:"exit"              yaml:"exit"`              // exit hive after command (bool or $ENV_VAR)
	Scope   []string           `json:"scope,omitempty"   yaml:"scope,omitempty"`   // views where command is active (empty = global)
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
			Command: `{{ agentCommand }} {{ agentFlags }}{{- if .Prompt }} {{ .Prompt | shq }}{{ end }}`,
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
			UpdateChecker: true,
		},
		Views: ViewsConfig{
			Sessions: SessionsViewConfig{
				RefreshInterval: 15 * time.Second,
				PreviewEnabled:  true,
			},
		},
		Messaging: MessagingConfig{
			TopicPrefix: "agent",
			MaxMessages: 100,
		},
		Todos: TodosConfig{
			Limiter: TodosLimiterConfig{
				MaxPending:          0,
				RateLimitPerSession: 0,
			},
			Notifications: TodosNotifyConfig{
				Toast: true,
			},
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

			// Backwards compat: migrate deprecated repo_dirs → workspaces
			if len(cfg.Workspaces) == 0 && len(cfg.RepoDirsCompat) > 0 {
				cfg.Workspaces = cfg.RepoDirsCompat
			}
			cfg.RepoDirsCompat = nil
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
	if c.Views.Sessions.GroupBy == "" {
		c.Views.Sessions.GroupBy = GroupByRepo
	}
	if c.CopyCommand == "" {
		c.CopyCommand = defaultCopyCommand()
	}
	if c.Tmux.PollInterval == 0 {
		c.Tmux.PollInterval = 1500 * time.Millisecond
	}
	if len(c.Tmux.PreviewWindowMatcher) == 0 {
		c.Tmux.PreviewWindowMatcher = []string{"claude", "aider", "codex", "cursor", "crush", "agent", "llm", "opencode"}
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
		c.validateGroupBy(),
		c.validateKeybindingsBasic(),
		c.validateUserCommandsBasic(),
		c.validateMaxRecycled(),
		c.validateAgents(),
		c.validateWindowsBasic(),
		c.validateTodos(),
		c.validateCloneStrategies(),
	)
}

// validateCloneStrategies checks clone_strategy on each rule.
func (c *Config) validateCloneStrategies() error {
	var errs criterio.FieldErrorsBuilder
	for i, rule := range c.Rules {
		if err := ValidateCloneStrategy(rule.CloneStrategy); err != nil {
			errs = errs.Append(fmt.Sprintf("rules[%d].clone_strategy", i), err)
		}
	}
	return errs.ToError()
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
				errs = errs.Append(field+".scope", fmt.Errorf("invalid scope %q: must be one of: %s", scope, strings.Join(ValidScopes, ", ")))
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

// ReposDir returns the path where cloned repositories are stored.
func (c *Config) ReposDir() string {
	return filepath.Join(c.DataDir, "repos")
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

// GetCloneStrategy returns the effective clone strategy for the given remote.
// The last matching rule with a clone_strategy set wins; defaults to "full".
func (c *Config) GetCloneStrategy(remote string) string {
	strategy := CloneStrategyFull
	for _, rule := range c.Rules {
		if rule.Matches(remote) && rule.CloneStrategy != "" {
			strategy = rule.CloneStrategy
		}
	}
	return strategy
}

// ValidateCloneStrategy returns an error if s is not a valid clone strategy value.
func ValidateCloneStrategy(s string) error {
	switch s {
	case "", CloneStrategyFull, CloneStrategyWorktree:
		return nil
	default:
		return fmt.Errorf("invalid clone_strategy %q: must be %q or %q", s, CloneStrategyFull, CloneStrategyWorktree)
	}
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
