// Package config handles configuration loading and validation for hive.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Built-in action names for keybindings.
const (
	ActionRecycle = "recycle"
	ActionDelete  = "delete"
)

// defaultKeybindings provides built-in keybindings that users can override.
var defaultKeybindings = map[string]Keybinding{
	"r": {
		Action:  ActionRecycle,
		Help:    "recycle",
		Confirm: "Are you sure you want to recycle this session?",
	},
	"d": {
		Action:  ActionDelete,
		Help:    "delete",
		Confirm: "Are you sure you want to delete this session?",
	},
}

// Config holds the application configuration.
type Config struct {
	Commands    Commands              `yaml:"commands"`
	Git         GitConfig             `yaml:"git"`
	GitPath     string                `yaml:"git_path"`
	Keybindings map[string]Keybinding `yaml:"keybindings"`
	Hooks       []Hook                `yaml:"hooks"`
	Templates   map[string]Template   `yaml:"templates"`
	DataDir     string                `yaml:"-"` // set by caller, not from config file
}

// FieldType represents the type of a template field.
type FieldType string

// Supported field types for template forms.
const (
	FieldTypeString      FieldType = "string"
	FieldTypeText        FieldType = "text"
	FieldTypeSelect      FieldType = "select"
	FieldTypeMultiSelect FieldType = "multi-select"
)

// Template defines a session template with form fields and prompt rendering.
type Template struct {
	Description string          `yaml:"description"`
	Fields      []TemplateField `yaml:"fields"`
	Name        string          `yaml:"name"`   // optional session name template
	Prompt      string          `yaml:"prompt"` // required prompt template
}

// TemplateField defines a single input field in a template form.
type TemplateField struct {
	Name        string        `yaml:"name"`        // variable name used in templates
	Label       string        `yaml:"label"`       // display label for the form
	Type        FieldType     `yaml:"type"`        // field type: string, text, select, multi-select
	Required    bool          `yaml:"required"`    // whether the field is required
	Default     string        `yaml:"default"`     // default value
	Placeholder string        `yaml:"placeholder"` // placeholder text for input fields
	Options     []FieldOption `yaml:"options"`     // options for select/multi-select fields
}

// FieldOption defines an option for select or multi-select fields.
type FieldOption struct {
	Value string `yaml:"value"` // the value stored when selected
	Label string `yaml:"label"` // display label shown in the form
}

// GitConfig holds git-related configuration.
type GitConfig struct {
	StatusWorkers int `yaml:"status_workers"`
}

// Hook defines setup commands for specific repositories.
type Hook struct {
	// Pattern matches against remote URL (supports glob patterns).
	Pattern string `yaml:"pattern"`
	// Commands to run in the session directory after clone/recycle.
	Commands []string `yaml:"commands"`
}

// Commands defines the shell commands used by hive.
type Commands struct {
	Spawn   []string `yaml:"spawn"`
	Recycle []string `yaml:"recycle"`
}

// Keybinding defines a TUI keybinding action.
type Keybinding struct {
	Action  string `yaml:"action"`  // built-in action name (recycle, delete)
	Help    string `yaml:"help"`    // help text shown in TUI
	Sh      string `yaml:"sh"`      // shell command template
	Confirm string `yaml:"confirm"` // confirmation prompt (empty = no confirm)
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Commands: Commands{
			Spawn:   []string{},
			Recycle: []string{"git reset --hard", "git checkout $(git rev-parse --abbrev-ref origin/HEAD | sed 's|origin/||')", "git pull"},
		},
		Git: GitConfig{
			StatusWorkers: 3,
		},
		GitPath:     "git",
		Keybindings: map[string]Keybinding{},
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
}

// mergeKeybindings merges user keybindings into defaults.
// User keybindings override defaults for the same key.
func mergeKeybindings(defaults, user map[string]Keybinding) map[string]Keybinding {
	result := make(map[string]Keybinding, len(defaults)+len(user))

	// Copy defaults first
	for k, v := range defaults {
		result[k] = v
	}

	// Override with user config
	for k, v := range user {
		result[k] = v
	}

	return result
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	if c.GitPath == "" {
		return fmt.Errorf("git_path cannot be empty")
	}

	if c.DataDir == "" {
		return fmt.Errorf("data directory cannot be empty")
	}

	if c.Git.StatusWorkers < 1 {
		return fmt.Errorf("git.status_workers must be at least 1")
	}

	for key, kb := range c.Keybindings {
		if kb.Action == "" && kb.Sh == "" {
			return fmt.Errorf("keybinding %q must have either action or sh", key)
		}
		if kb.Action != "" && kb.Sh != "" {
			return fmt.Errorf("keybinding %q cannot have both action and sh", key)
		}
		if kb.Action != "" {
			if !isValidAction(kb.Action) {
				return fmt.Errorf("keybinding %q has invalid action %q", key, kb.Action)
			}
		}
	}

	for name, tmpl := range c.Templates {
		if err := tmpl.Validate(name); err != nil {
			return err
		}
	}

	return nil
}

// Validate checks that a template definition is valid.
func (t *Template) Validate(name string) error {
	if t.Prompt == "" {
		return fmt.Errorf("template %q: prompt is required", name)
	}

	fieldNames := make(map[string]bool)
	for i, field := range t.Fields {
		if field.Name == "" {
			return fmt.Errorf("template %q: field %d: name is required", name, i)
		}

		if fieldNames[field.Name] {
			return fmt.Errorf("template %q: duplicate field name %q", name, field.Name)
		}
		fieldNames[field.Name] = true

		if err := field.Validate(name); err != nil {
			return err
		}
	}

	return nil
}

// Validate checks that a template field definition is valid.
func (f *TemplateField) Validate(templateName string) error {
	if !f.Type.IsValid() {
		return fmt.Errorf("template %q: field %q: invalid type %q", templateName, f.Name, f.Type)
	}

	if f.Type == FieldTypeSelect || f.Type == FieldTypeMultiSelect {
		if len(f.Options) == 0 {
			return fmt.Errorf("template %q: field %q: select fields require options", templateName, f.Name)
		}
	}

	return nil
}

// IsValid checks if the field type is a supported type.
func (ft FieldType) IsValid() bool {
	switch ft {
	case FieldTypeString, FieldTypeText, FieldTypeSelect, FieldTypeMultiSelect:
		return true
	default:
		return false
	}
}

// ReposDir returns the path where cloned repositories are stored.
func (c *Config) ReposDir() string {
	return filepath.Join(c.DataDir, "repos")
}

// SessionsFile returns the path to the sessions JSON file.
func (c *Config) SessionsFile() string {
	return filepath.Join(c.DataDir, "sessions.json")
}

func isValidAction(action string) bool {
	switch action {
	case ActionRecycle, ActionDelete:
		return true
	default:
		return false
	}
}
