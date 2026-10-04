// Package config loads and validates the engine sections of hive's
// config.yaml: the settings the hive engine reads, which both the CLI and Hive
// Desktop run on. Sections only the CLI reads live in cmd/hive/internal/config,
// which embeds this Config.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/colonyops/hive/pkg/pathutil"
	"github.com/hay-kot/criterio"
	"gopkg.in/yaml.v3"
)

// Clone strategy constants.
const (
	CloneStrategyFull     = "full"
	CloneStrategyWorktree = "worktree"
)

const (
	// EnvDefaultAgent overrides agents.default when set.
	EnvDefaultAgent = "HIVE_DEFAULT_AGENT"
	// EnvContextBaseDir overrides context.base_dir when set.
	EnvContextBaseDir = "HIVE_CONTEXT_BASE_DIR"
	// EnvGitPath overrides git_path when set.
	EnvGitPath = "HIVE_GIT_PATH"
)

// Config holds the engine configuration. Decoding is not strict, so a file
// that also carries CLI-only sections loads without error.
type Config struct {
	Git                 GitConfig       `json:"git"                   yaml:"git"`
	GitPath             string          `json:"git_path"              yaml:"git_path"`
	Rules               []Rule          `json:"rules"                 yaml:"rules"`
	Agents              AgentsConfig    `json:"agents"                yaml:"agents"`
	AutoDeleteCorrupted bool            `json:"auto_delete_corrupted" yaml:"auto_delete_corrupted"`
	Context             ContextConfig   `json:"context"               yaml:"context"`
	Messaging           MessagingConfig `json:"messaging"             yaml:"messaging"`
	Tmux                TmuxConfig      `json:"tmux"                  yaml:"tmux"`
	Terminal            TerminalConfig  `json:"terminal"              yaml:"terminal"`
	Database            DatabaseConfig  `json:"database"              yaml:"database"`
	Todos               TodosConfig     `json:"todos"                 yaml:"todos"`
	Workspaces          []string        `json:"workspaces"            yaml:"workspaces"` // parent directories containing git repository folders for new session dialog
	RepoDirsCompat      []string        `json:"-"                     yaml:"repo_dirs"`  // deprecated: use workspaces instead (kept for backwards compatibility)
	DataDir             string          `json:"-"                     yaml:"-"`          // set by caller, not from config file
}

// AgentsConfig holds agent profile configuration.
// The "default" key selects which profile to use; all other keys are profile definitions.
type AgentsConfig struct {
	Default       string                  // reserved key selecting the active profile
	AgentSelector bool                    // when true, the TUI new-session form prompts for agent selection
	Profiles      map[string]AgentProfile // profile name → agent configuration
}

// UnmarshalYAML separates the "default" and "agent_selector" keys from profile entries.
func (a *AgentsConfig) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("agents: expected mapping, got %v", node.Kind)
	}

	a.Profiles = make(map[string]AgentProfile)

	for i := 0; i < len(node.Content)-1; i += 2 {
		key := node.Content[i].Value
		val := node.Content[i+1]

		switch key {
		case "default":
			var s string
			if err := val.Decode(&s); err != nil {
				return fmt.Errorf("agents.default: %w", err)
			}
			a.Default = s
		case "agent_selector":
			var b bool
			if err := val.Decode(&b); err != nil {
				return fmt.Errorf("agents.agent_selector: %w", err)
			}
			a.AgentSelector = b
		default:
			var profile AgentProfile
			if err := val.Decode(&profile); err != nil {
				return fmt.Errorf("agents.%s: %w", key, err)
			}
			a.Profiles[key] = profile
		}
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
// Individual flags are NOT quoted; the caller is responsible for quoting
// the entire value (e.g., via shq in templates) or relying on shell word
// splitting when consuming the value.
func (p AgentProfile) ShellFlags() string {
	return strings.Join(p.Flags, " ")
}

// ContextConfig configures context directory behavior.
type ContextConfig struct {
	BaseDir     string `json:"base_dir"     yaml:"base_dir"`     // override context base path (default: $HIVE_DATA_DIR/context/)
	SymlinkName string `json:"symlink_name" yaml:"symlink_name"` // default: ".hive"
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

// TerminalConfig holds integration-agnostic terminal status settings.
// Transport-specific knobs (poll cadence, capture) stay under tmux:.
type TerminalConfig struct {
	Status TerminalStatusConfig `json:"status" yaml:"status"`
}

// TerminalStatusConfig configures the Stage 2 debounce tracker
// (internal/domain/terminal/status.Tracker).
type TerminalStatusConfig struct {
	Confirm TerminalConfirmConfig `json:"confirm" yaml:"confirm"`
}

// TerminalConfirmConfig holds one confirmation policy per status transition.
type TerminalConfirmConfig struct {
	Idle     ConfirmPolicyConfig `json:"idle"     yaml:"idle"`
	Missing  MissingPolicyConfig `json:"missing"  yaml:"missing"`
	Approval ConfirmPolicyConfig `json:"approval" yaml:"approval"`
}

// ConfirmPolicyConfig is the YAML/JSON shape of one status.ConfirmPolicy.
type ConfirmPolicyConfig struct {
	Polls         int           `json:"polls"          yaml:"polls"`
	MinDuration   time.Duration `json:"min_duration"   yaml:"min_duration"`
	StableContent *bool         `json:"stable_content" yaml:"stable_content"` // nil = default
}

// MissingPolicyConfig is deliberately polls-only: missing is decided by the
// tmux transport counting consecutive list-panes failures (polls N tolerates
// N-1 failures), never by the tracker's duration/content-stability debounce,
// so min_duration and stable_content have no meaning here — the narrower
// shape is what keeps them unconfigurable.
type MissingPolicyConfig struct {
	Polls int `json:"polls" yaml:"polls"`
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

// PaneConfig defines a tmux pane to create inside a window.
type PaneConfig struct {
	Command string `json:"command,omitempty" yaml:"command,omitempty"` // Command to run (template string, empty = shell)
	Dir     string `json:"dir,omitempty"     yaml:"dir,omitempty"`     // Working directory override (template string)
	Size    string `json:"size,omitempty"    yaml:"size,omitempty"`    // Pane size passed to tmux -l (for example "30%")
	Split   string `json:"split,omitempty"   yaml:"split,omitempty"`   // Split direction: horizontal or vertical (default vertical)
}

// WindowConfig defines a tmux window to create when spawning a session.
type WindowConfig struct {
	Name    string       `json:"name"              yaml:"name"`              // Window name (template string, required)
	Command string       `json:"command,omitempty" yaml:"command,omitempty"` // Command to run (template string, empty = shell); mutually exclusive with Panes
	Dir     string       `json:"dir,omitempty"     yaml:"dir,omitempty"`     // Working directory override (template string)
	Focus   bool         `json:"focus,omitempty"   yaml:"focus,omitempty"`   // Select this window after creation
	Panes   []PaneConfig `json:"panes,omitempty"   yaml:"panes,omitempty"`   // Panes to create in this window; mutually exclusive with Command
}

// Rule defines actions to take for matching repositories.
type Rule struct {
	// Pattern matches against remote URL (regex). Empty = matches all.
	Pattern string `json:"pattern" yaml:"pattern"`
	// Agent selects the agent profile used for matching repositories.
	Agent string `json:"agent,omitempty" yaml:"agent,omitempty"`
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
	// BranchTemplate is a Go template for the git branch name when using the worktree
	// clone strategy. Available variables: .Name, .Slug, .Owner, .Repo, .ID.
	// Defaults to "hive/{{ .Slug }}-{{ .ID }}" when empty.
	BranchTemplate string `json:"branch_template,omitempty" yaml:"branch_template,omitempty"`
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
		AutoDeleteCorrupted: true,
		Context: ContextConfig{
			SymlinkName: ".hive",
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

	data, err := ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	if data != nil {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parse config file: %w", err)
		}
	}

	cfg.Complete(dataDir)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// ReadFile returns the config file's contents, or nil when configPath is
// empty or names no file.
func ReadFile(configPath string) ([]byte, error) {
	if configPath == "" {
		return nil, nil
	}
	if _, err := os.Stat(configPath); err != nil {
		return nil, nil
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}
	return data, nil
}

// Complete finishes a decoded Config: it sets the data directory, adopts the
// deprecated repo_dirs key, fills defaults for unset values and applies
// environment overrides. Load calls it; a program that decodes the file into
// its own struct calls it on the embedded Config before validating.
func (c *Config) Complete(dataDir string) {
	c.DataDir = dataDir
	if len(c.Workspaces) == 0 && len(c.RepoDirsCompat) > 0 {
		c.Workspaces = c.RepoDirsCompat
	}
	c.RepoDirsCompat = nil
	c.applyDefaults()
	c.applyEnvironmentOverrides()
}

// applyDefaults sets default values for any unset configuration options.
func (c *Config) applyDefaults() {
	defaults := DefaultConfig()
	if c.Git.StatusWorkers == 0 {
		c.Git.StatusWorkers = defaults.Git.StatusWorkers
	}
	if c.Context.SymlinkName == "" {
		c.Context.SymlinkName = defaults.Context.SymlinkName
	}
	if c.Tmux.PollInterval == 0 {
		c.Tmux.PollInterval = 1500 * time.Millisecond
	}
	if len(c.Tmux.PreviewWindowMatcher) == 0 {
		c.Tmux.PreviewWindowMatcher = []string{"claude", "gemini", "aider", "codex", "cursor", "crush", "cline", "opencode", "pi", "agent", "llm"}
	}
	c.applyTerminalConfirmDefaults()
	if c.Database.MaxOpenConns == 0 {
		c.Database.MaxOpenConns = 2
	}
	if c.Database.MaxIdleConns == 0 {
		c.Database.MaxIdleConns = 2
	}
	if c.Database.BusyTimeout == 0 {
		c.Database.BusyTimeout = 5000
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

// applyTerminalConfirmDefaults fills unset (zero) confirm-policy fields.
// Polls == 0 is the "unset" sentinel here, same as MaxRecycled's "0 means
// unlimited" pattern elsewhere in this file — a user-set negative value is
// the only way to express an actually-invalid policy past this point (see
// Validate).
func (c *Config) applyTerminalConfirmDefaults() {
	confirm := &c.Terminal.Status.Confirm

	if confirm.Idle.Polls == 0 {
		confirm.Idle.Polls = 2
	}
	if confirm.Idle.MinDuration == 0 {
		confirm.Idle.MinDuration = 2 * time.Second
	}
	if confirm.Idle.StableContent == nil {
		stableDefault := true
		confirm.Idle.StableContent = &stableDefault
	}

	if confirm.Missing.Polls == 0 {
		confirm.Missing.Polls = 2
	}

	if confirm.Approval.Polls == 0 {
		confirm.Approval.Polls = 1
	}
}

func (c *Config) applyEnvironmentOverrides() {
	overrides := []struct {
		env    string
		target *string
	}{
		{env: EnvDefaultAgent, target: &c.Agents.Default},
		{env: EnvContextBaseDir, target: &c.Context.BaseDir},
		{env: EnvGitPath, target: &c.GitPath},
	}

	for _, override := range overrides {
		if value := os.Getenv(override.env); value != "" {
			*override.target = value
		}
	}
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
		c.validateMaxRecycled(),
		c.validateAgents(),
		c.validateWindowsBasic(),
		c.validateTodos(),
		c.validateCloneStrategies(),
		c.validateTerminalConfirm(),
	)
}

// validateTerminalConfirm rejects negative confirm-policy values. Polls == 0
// is deliberately accepted: applyDefaults treats it as "unset" and fills in
// the documented default before Validate ever runs it through Load, so 0 is
// never distinguishable from "not configured". A negative value survives
// applyDefaults untouched (its zero-check doesn't match) and is the only
// invalid state actually expressible at this point.
func (c *Config) validateTerminalConfirm() error {
	var errs criterio.FieldErrorsBuilder

	check := func(field string, p ConfirmPolicyConfig) {
		if p.Polls < 0 {
			errs = errs.Append(field+".polls", fmt.Errorf("must be >= 0, got %d", p.Polls))
		}
		if p.MinDuration < 0 {
			errs = errs.Append(field+".min_duration", fmt.Errorf("must be >= 0, got %s", p.MinDuration))
		}
	}

	check("terminal.status.confirm.idle", c.Terminal.Status.Confirm.Idle)
	check("terminal.status.confirm.approval", c.Terminal.Status.Confirm.Approval)

	if c.Terminal.Status.Confirm.Missing.Polls < 0 {
		errs = errs.Append("terminal.status.confirm.missing.polls", fmt.Errorf("must be >= 0, got %d", c.Terminal.Status.Confirm.Missing.Polls))
	}

	return errs.ToError()
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

		// Each window must have a name and cannot mix command with panes.
		for j, w := range rule.Windows {
			field := fmt.Sprintf("rules[%d].windows[%d]", i, j)
			if w.Name == "" {
				errs = errs.Append(field+".name", fmt.Errorf("is required"))
			}
			if w.Command != "" && len(w.Panes) > 0 {
				errs = errs.Append(field, fmt.Errorf("command and panes are mutually exclusive"))
			}
			for k, p := range w.Panes {
				if p.Split != "" && p.Split != "horizontal" && p.Split != "vertical" {
					errs = errs.Append(fmt.Sprintf("%s.panes[%d].split", field, k), fmt.Errorf("must be one of: horizontal, vertical"))
				}
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

// validateAgents checks that configured agent references point at existing profiles.
func (c *Config) validateAgents() error {
	var errs criterio.FieldErrorsBuilder
	if _, ok := c.Agents.Profiles[c.Agents.Default]; !ok {
		errs = errs.Append("agents.default", fmt.Errorf("profile %q not found in agents config", c.Agents.Default))
	}
	for i, rule := range c.Rules {
		if rule.Agent == "" {
			continue
		}
		if _, ok := c.Agents.Profiles[rule.Agent]; !ok {
			errs = errs.Append(fmt.Sprintf("rules[%d].agent", i), fmt.Errorf("profile %q not found in agents config", rule.Agent))
		}
	}
	return errs.ToError()
}

// ReposDir returns the path where cloned repositories are stored.
func (c *Config) ReposDir() string {
	return filepath.Join(c.DataDir, "repos")
}

// ContextDir returns the base context directory path.
func (c *Config) ContextDir() string {
	if c.Context.BaseDir != "" {
		return pathutil.ExpandHome(c.Context.BaseDir)
	}
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

// GetBranchTemplate returns the branch_template for the given remote URL.
// The last matching rule with a branch_template set wins.
// Returns "" if no rule defines a template (caller uses the default "hive-<id>" branch).
func (c *Config) GetBranchTemplate(remote string) string {
	var tmpl string
	for _, rule := range c.Rules {
		if rule.Matches(remote) && rule.BranchTemplate != "" {
			tmpl = rule.BranchTemplate
		}
	}
	return tmpl
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
	Agent    string
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
		if rule.Agent != "" {
			strategy.Agent = rule.Agent
		}
		switch {
		case len(rule.Windows) > 0:
			strategy.Windows = rule.Windows
			strategy.Commands = nil
		case batch && len(rule.BatchSpawn) > 0:
			strategy.Windows = nil
			strategy.Commands = rule.BatchSpawn
		case !batch && len(rule.Spawn) > 0:
			strategy.Windows = nil
			strategy.Commands = rule.Spawn
		}
	}
	if !strategy.IsWindows() && len(strategy.Commands) == 0 {
		strategy.Windows = DefaultWindows()
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
