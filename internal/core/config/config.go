// Package config handles configuration loading and validation for hive.
package config

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

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

// Built-in action names for UserCommands.
const (
	ActionRecycle        = "recycle"
	ActionDelete         = "delete"
	ActionFilterAll      = "filter-all"
	ActionFilterActive   = "filter-active"
	ActionFilterApproval = "filter-approval"
	ActionFilterReady    = "filter-ready"
)

// defaultUserCommands provides built-in commands that users can override.
var defaultUserCommands = map[string]UserCommand{
	"Recycle": {
		Action:  ActionRecycle,
		Help:    "recycle",
		Confirm: "Are you sure you want to recycle this session?",
	},
	"Delete": {
		Action:  ActionDelete,
		Help:    "delete",
		Confirm: "Are you sure you want to delete this session?",
	},
	"FilterAll": {
		Action: ActionFilterAll,
		Help:   "show all sessions",
		Silent: true,
	},
	"FilterActive": {
		Action: ActionFilterActive,
		Help:   "show sessions with active agents",
		Silent: true,
	},
	"FilterApproval": {
		Action: ActionFilterApproval,
		Help:   "show sessions needing approval",
		Silent: true,
	},
	"FilterReady": {
		Action: ActionFilterReady,
		Help:   "show sessions with idle agents",
		Silent: true,
	},
}

// defaultKeybindings provides built-in keybindings that users can override.
var defaultKeybindings = map[string]Keybinding{
	"r": {Cmd: "Recycle"},
	"d": {Cmd: "Delete"},
}

// CurrentConfigVersion is the latest config schema version.
// Increment this when making breaking changes to config format.
const CurrentConfigVersion = "0.2.4"

// Config holds the application configuration.
type Config struct {
	Version             string                 `yaml:"version"`
	Commands            Commands               `yaml:"commands"`
	Git                 GitConfig              `yaml:"git"`
	GitPath             string                 `yaml:"git_path"`
	Keybindings         map[string]Keybinding  `yaml:"keybindings"`
	UserCommands        map[string]UserCommand `yaml:"usercommands"`
	Rules               []Rule                 `yaml:"rules"`
	AutoDeleteCorrupted bool                   `yaml:"auto_delete_corrupted"`
	History             HistoryConfig          `yaml:"history"`
	Context             ContextConfig          `yaml:"context"`
	TUI                 TUIConfig              `yaml:"tui"`
	Messaging           MessagingConfig        `yaml:"messaging"`
	Integrations        IntegrationsConfig     `yaml:"integrations"`
	Database            DatabaseConfig         `yaml:"database"`
	RepoDirs            []string               `yaml:"repo_dirs"` // directories containing git repositories for new session dialog
	DataDir             string                 `yaml:"-"`         // set by caller, not from config file
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
	RefreshInterval time.Duration `yaml:"refresh_interval"` // default: 15s, 0 to disable
	PreviewEnabled  bool          `yaml:"preview_enabled"`  // enable tmux pane preview sidebar
}

// MessagingConfig holds messaging-related configuration.
type MessagingConfig struct {
	TopicPrefix string `yaml:"topic_prefix"` // default: "agent"
	MaxMessages int    `yaml:"max_messages"` // max messages per topic (default: 100, 0 = unlimited)
}

// IntegrationsConfig holds configuration for external integrations.
type IntegrationsConfig struct {
	Terminal TerminalConfig `yaml:"terminal"`
}

// DatabaseConfig holds SQLite database configuration.
type DatabaseConfig struct {
	MaxOpenConns int `yaml:"max_open_conns"` // max open connections (default: 2)
	MaxIdleConns int `yaml:"max_idle_conns"` // max idle connections (default: 2)
	BusyTimeout  int `yaml:"busy_timeout"`   // busy timeout in milliseconds (default: 5000)
}

// TerminalConfig holds terminal multiplexer integration configuration.
type TerminalConfig struct {
	Enabled              []string      `yaml:"enabled"`                // list of enabled integrations, e.g. ["tmux"]
	PollInterval         time.Duration `yaml:"poll_interval"`          // status check frequency, default 1.5s
	PreviewWindowMatcher []string      `yaml:"preview_window_matcher"` // regex patterns for preferred window names (e.g., ["claude", "aider"])
}

// IsEnabled returns true if the given integration name is in the enabled list.
func (t TerminalConfig) IsEnabled(name string) bool {
	return slices.Contains(t.Enabled, name)
}

// GitConfig holds git-related configuration.
type GitConfig struct {
	StatusWorkers int `yaml:"status_workers"`
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
	// Spawn commands to run when creating a new session (hive new).
	Spawn []string `yaml:"spawn,omitempty"`
	// BatchSpawn commands to run when creating a batch session (hive batch).
	BatchSpawn []string `yaml:"batch_spawn,omitempty"`
}

// Commands defines the shell commands used by hive.
type Commands struct {
	Recycle     []string `yaml:"recycle"`
	CopyCommand string   `yaml:"copy_command"` // command to copy to clipboard (e.g., pbcopy, xclip)
}

// Keybinding defines a TUI keybinding that references a UserCommand.
type Keybinding struct {
	Cmd     string `yaml:"cmd"`     // command name (required, references UserCommand)
	Help    string `yaml:"help"`    // optional override for help text
	Confirm string `yaml:"confirm"` // optional override for confirmation prompt
}

// UserCommand defines a named command accessible via command palette or keybindings.
type UserCommand struct {
	Action  string `yaml:"action"`  // built-in action (recycle, delete) - mutually exclusive with sh
	Sh      string `yaml:"sh"`      // shell command template - mutually exclusive with action
	Help    string `yaml:"help"`    // description shown in palette/help
	Confirm string `yaml:"confirm"` // confirmation prompt (empty = no confirm)
	Silent  bool   `yaml:"silent"`  // skip loading popup for fast commands
	Exit    string `yaml:"exit"`    // exit hive after command (bool or $ENV_VAR)
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
		Commands: Commands{
			Recycle: []string{
				"git fetch origin",
				"git checkout {{ .DefaultBranch }}",
				"git reset --hard origin/{{ .DefaultBranch }}",
				"git clean -fd",
			},
		},
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
	if c.Commands.CopyCommand == "" {
		c.Commands.CopyCommand = defaultCopyCommand()
	}
	if c.Integrations.Terminal.PollInterval == 0 {
		c.Integrations.Terminal.PollInterval = 1500 * time.Millisecond
	}
	if len(c.Integrations.Terminal.PreviewWindowMatcher) == 0 {
		c.Integrations.Terminal.PreviewWindowMatcher = []string{"claude", "aider", "codex"}
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

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	return criterio.ValidateStruct(
		criterio.Run("git_path", c.GitPath, criterio.Required[string]),
		criterio.Run("data_dir", c.DataDir, criterio.Required[string]),
		criterio.Run("git.status_workers", c.Git.StatusWorkers, criterio.Min(1)),
		criterio.Run("database.max_open_conns", c.Database.MaxOpenConns, criterio.Min(1)),
		criterio.Run("database.max_idle_conns", c.Database.MaxIdleConns, criterio.Min(1)),
		criterio.Run("database.busy_timeout", c.Database.BusyTimeout, criterio.Min(0)),
		c.validateKeybindingsBasic(),
		c.validateUserCommandsBasic(),
		c.validateMaxRecycled(),
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

		// Must have exactly one of action or sh
		if cmd.Action == "" && cmd.Sh == "" {
			errs = errs.Append(field, fmt.Errorf("must have either action or sh"))
			continue
		}
		if cmd.Action != "" && cmd.Sh != "" {
			errs = errs.Append(field, fmt.Errorf("cannot have both action and sh"))
			continue
		}
		if cmd.Action != "" && !isValidAction(cmd.Action) {
			errs = errs.Append(field, fmt.Errorf("invalid action %q", cmd.Action))
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

// validateKeybindingsBasic performs basic keybinding validation for the Validate() method.
// Keybindings must reference existing UserCommands (including system defaults).
func (c *Config) validateKeybindingsBasic() error {
	var errs criterio.FieldErrorsBuilder

	// Build merged commands map (user commands + system defaults)
	allCommands := c.MergedUserCommands()

	for key, kb := range c.Keybindings {
		field := fmt.Sprintf("keybindings[%q]", key)

		if kb.Cmd == "" {
			errs = errs.Append(field, fmt.Errorf("cmd is required"))
			continue
		}

		// Validate that cmd references an existing command
		if _, exists := allCommands[kb.Cmd]; !exists {
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

func isValidAction(action string) bool {
	switch action {
	case ActionRecycle, ActionDelete:
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

// GetSpawnCommands returns the spawn commands for the given remote URL.
// If batch is true, returns BatchSpawn commands; otherwise returns Spawn commands.
// Rules are evaluated in order; the last matching rule with spawn commands wins.
func (c *Config) GetSpawnCommands(remote string, batch bool) []string {
	var result []string
	for _, rule := range c.Rules {
		if rule.Pattern == "" || matchesPattern(rule.Pattern, remote) {
			if batch && len(rule.BatchSpawn) > 0 {
				result = rule.BatchSpawn
			} else if !batch && len(rule.Spawn) > 0 {
				result = rule.Spawn
			}
		}
	}
	return result
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
