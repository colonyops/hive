package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"github.com/colonyops/hive/internal/sources"
	"github.com/colonyops/hive/pkg/pathutil"
	"github.com/colonyops/hive/pkg/tmpl"
	"github.com/hay-kot/criterio"
)

// SpawnTemplateData defines available fields for spawn command templates (hive new).
type SpawnTemplateData struct {
	Path       string // Absolute path to the session directory
	Name       string // Session name (directory basename)
	Slug       string // Session slug (URL-safe version of name)
	ContextDir string // Path to context directory
	Owner      string // Repository owner
	Repo       string // Repository name
	ID         string // Short random ID shared with the session directory
}

// BatchSpawnTemplateData defines available fields for batch_spawn command templates (hive batch).
type BatchSpawnTemplateData struct {
	Path       string // Absolute path to the session directory
	Name       string // Session name (directory basename)
	Prompt     string // User-provided prompt (batch only)
	Slug       string // Session slug (URL-safe version of name)
	ContextDir string // Path to context directory
	Owner      string // Repository owner
	Repo       string // Repository name
	ID         string // Short random ID shared with the session directory
}

// RecycleTemplateData defines available fields for recycle command templates.
type RecycleTemplateData struct {
	DefaultBranch string // Default branch name (e.g., "main" or "master")
}

// BranchTemplateData defines available fields for branch_template Go templates.
type BranchTemplateData struct {
	Name  string // Session name (display name)
	Slug  string // Session slug (URL-safe version of name)
	Owner string // Repository owner
	Repo  string // Repository name
	ID    string // Short random ID shared with the session directory
}

// SourceTemplateData defines available fields for source session
// templates (name/prompt/tags). Fields is a map because item field names are
// dynamic per-source; a missing .Fields.<key> is a render-time error, not
// a config-time one.
type SourceTemplateData struct {
	ID       string
	Title    string
	Subtitle string
	Detail   string
	Fields   map[string]any
}

// validateSources checks the top-level sources config template syntax and
// host-backend overrides.
func (c *Config) validateSources() error {
	var errs criterio.FieldErrorsBuilder

	if err := validateSourceTemplateSet("sources.issues.templates", c.Sources.Issues.Templates); err != nil {
		errs = errs.Append("sources.issues.templates", err)
	}
	if err := validateSourceTemplateSet("sources.prs.templates", c.Sources.PRs.Templates); err != nil {
		errs = errs.Append("sources.prs.templates", err)
	}
	for host, backend := range c.Sources.Hosts {
		if _, err := sources.ParseBackend(backend); err != nil {
			errs = errs.Append("sources.hosts."+host, fmt.Errorf("invalid backend %q: expected one of %v", backend, sources.BackendNames()))
		}
	}

	return errs.ToError()
}

// validateSourceTemplateSet syntax-checks the name/prompt/tags templates
// of a single source's SourceTemplateConfig.
func validateSourceTemplateSet(field string, cfg SourceTemplateConfig) error {
	if err := validateSourceTemplate(cfg.Name); err != nil {
		return fmt.Errorf("%s.name: %w", field, err)
	}
	if err := validateSourceTemplate(cfg.Prompt); err != nil {
		return fmt.Errorf("%s.prompt: %w", field, err)
	}
	for i, tag := range cfg.Tags {
		if err := validateSourceTemplate(tag); err != nil {
			return fmt.Errorf("%s.tags[%d]: %w", field, i, err)
		}
	}
	return nil
}

// validateSourceTemplate syntax-checks a single source template.
// Item data (.Fields.<key>) is only known at selection time, so missing
// keys are a render-time concern, not a config error.
func validateSourceTemplate(tmplStr string) error {
	if tmplStr == "" {
		return nil
	}
	return validationRenderer.ValidateSyntax(tmplStr)
}

// ValidationWarning represents a non-fatal configuration issue.
type ValidationWarning struct {
	Category string `json:"category"`
	Item     string `json:"item,omitempty"`
	Message  string `json:"message"`
}

// ValidateDeep performs comprehensive validation of the configuration including
// template syntax, regex patterns, and file accessibility. The configPath argument
// specifies the config file location to validate (empty string skips config file check).
// This calls Validate() first for basic structural validation, then adds I/O checks.
func (c *Config) ValidateDeep(configPath string) error {
	if err := c.Validate(); err != nil {
		return err
	}

	return criterio.ValidateStruct(
		c.validateFileAccess(configPath),
		c.validateContextBaseDir(),
		c.validateRules(),
		c.validateUserCommandTemplates(),
	)
}

// Warnings returns non-fatal configuration issues.
func (c *Config) Warnings() []ValidationWarning {
	var warnings []ValidationWarning

	if c.hasLegacyKeybindings {
		warnings = append(warnings, ValidationWarning{
			Category: "Keybindings",
			Message:  "top-level 'keybindings' field is deprecated; move entries to 'views.sessions.keybindings'",
		})
	}

	for i, rule := range c.Rules {
		if len(rule.Commands) == 0 && len(rule.Copy) == 0 {
			warnings = append(warnings, ValidationWarning{
				Category: "Rules",
				Item:     fmt.Sprintf("rule %d", i),
				Message:  "rule has neither commands nor copy defined",
			})
		}
	}

	return warnings
}

// validateFileAccess checks config file, data directory, and git executable.
func (c *Config) validateFileAccess(configPath string) error {
	return criterio.ValidateStruct(
		validateConfigFile(configPath),
		criterio.Run("git_path", c.GitPath, gitExecutableExists),
		criterio.Run("data_dir", c.DataDir, isDirectoryOrNotExist),
	)
}

func validateConfigFile(configPath string) error {
	if configPath == "" {
		return nil
	}

	info, err := os.Stat(configPath)
	if os.IsNotExist(err) {
		return nil // not found is fine, using defaults
	}
	if err != nil {
		return criterio.NewFieldErrors("config_file", fmt.Errorf("cannot access: %w", err))
	}
	if info.IsDir() {
		return criterio.NewFieldErrors("config_file", fmt.Errorf("%s is a directory, not a file", configPath))
	}
	return nil
}

// gitExecutableExists validates that the git path is executable.
func gitExecutableExists(path string) error {
	if path == "" {
		return nil
	}
	if _, err := exec.LookPath(path); err != nil {
		return fmt.Errorf("executable not found: %s", path)
	}
	return nil
}

// isDirectoryOrNotExist validates that a path is a directory or doesn't exist.
func isDirectoryOrNotExist(path string) error {
	if path == "" {
		return nil
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil // will be created
	}
	if err != nil {
		return fmt.Errorf("cannot access: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("exists but is not a directory")
	}
	return nil
}

// validateContextBaseDir checks that context.base_dir is an absolute or tilde path
// and that the expanded directory exists (or doesn't exist yet).
func (c *Config) validateContextBaseDir() error {
	dir := c.Context.BaseDir
	if dir == "" {
		return nil
	}

	expanded := pathutil.ExpandHome(dir)
	if !filepath.IsAbs(expanded) {
		return criterio.NewFieldErrors("context.base_dir", fmt.Errorf("must be an absolute path or start with ~/, got %q", dir))
	}
	return criterio.Run("context.base_dir", expanded, isDirectoryOrNotExist)
}

// validateRules checks rule patterns are valid regex and command templates are valid.
func (c *Config) validateRules() error {
	var errs criterio.FieldErrorsBuilder
	for i, rule := range c.Rules {
		if rule.Pattern != "" {
			if _, err := regexp.Compile(rule.Pattern); err != nil {
				errs = errs.Append(fmt.Sprintf("rules[%d].pattern", i), fmt.Errorf("invalid regex %q: %w", rule.Pattern, err))
			}
		}

		// Validate spawn templates
		for j, cmd := range rule.Spawn {
			if err := validateTemplate(cmd, SpawnTemplateData{}); err != nil {
				errs = errs.Append(fmt.Sprintf("rules[%d].spawn[%d]", i, j), fmt.Errorf("template error: %w", err))
			}
		}
		for j, cmd := range rule.BatchSpawn {
			if err := validateTemplate(cmd, BatchSpawnTemplateData{}); err != nil {
				errs = errs.Append(fmt.Sprintf("rules[%d].batch_spawn[%d]", i, j), fmt.Errorf("template error: %w", err))
			}
		}
		// Validate recycle templates
		for j, cmd := range rule.Recycle {
			if err := validateTemplate(cmd, RecycleTemplateData{}); err != nil {
				errs = errs.Append(fmt.Sprintf("rules[%d].recycle[%d]", i, j), fmt.Errorf("template error: %w", err))
			}
		}
		// Validate window templates
		for j, w := range rule.Windows {
			prefix := fmt.Sprintf("rules[%d].windows[%d]", i, j)
			if err := validateTemplate(w.Name, BatchSpawnTemplateData{}); err != nil {
				errs = errs.Append(prefix+".name", fmt.Errorf("template error: %w", err))
			}
			if w.Command != "" {
				if err := validateTemplate(w.Command, BatchSpawnTemplateData{}); err != nil {
					errs = errs.Append(prefix+".command", fmt.Errorf("template error: %w", err))
				}
			}
			if w.Dir != "" {
				if err := validateTemplate(w.Dir, BatchSpawnTemplateData{}); err != nil {
					errs = errs.Append(prefix+".dir", fmt.Errorf("template error: %w", err))
				}
			}
			for k, p := range w.Panes {
				panePrefix := fmt.Sprintf("%s.panes[%d]", prefix, k)
				if p.Command != "" {
					if err := validateTemplate(p.Command, BatchSpawnTemplateData{}); err != nil {
						errs = errs.Append(panePrefix+".command", fmt.Errorf("template error: %w", err))
					}
				}
				if p.Dir != "" {
					if err := validateTemplate(p.Dir, BatchSpawnTemplateData{}); err != nil {
						errs = errs.Append(panePrefix+".dir", fmt.Errorf("template error: %w", err))
					}
				}
				if p.Size != "" {
					if err := validateTemplate(p.Size, BatchSpawnTemplateData{}); err != nil {
						errs = errs.Append(panePrefix+".size", fmt.Errorf("template error: %w", err))
					}
				}
			}
		}
		// Validate branch_template
		if rule.BranchTemplate != "" {
			if err := validateTemplate(rule.BranchTemplate, BranchTemplateData{}); err != nil {
				errs = errs.Append(fmt.Sprintf("rules[%d].branch_template", i), fmt.Errorf("template error: %w", err))
			}
		}
	}
	return errs.ToError()
}

// validateUserCommandTemplates checks template syntax for usercommand shell commands.
// Basic usercommand structure validation is done by Validate().
func (c *Config) validateUserCommandTemplates() error {
	var errs criterio.FieldErrorsBuilder
	for name, cmd := range c.UserCommands {
		field := fmt.Sprintf("usercommands[%q]", name)
		errs = append(errs, ValidateUserCommandTemplates(field, cmd)...)
	}
	return errs.ToError()
}

// buildValidationData constructs test data for template validation.
// Fixed fields are always present; form fields are added under the Form key.
func buildValidationData(formFields []FormField) map[string]any {
	data := map[string]any{
		"Path":       "/tmp/test",
		"Remote":     "https://github.com/test/repo",
		"ID":         "test123",
		"Name":       "test-session",
		"Tool":       "claude",
		"TmuxWindow": "main",
		"Args":       []string{"arg1", "arg2"},
		"Doc": map[string]any{
			"Path":    "/tmp/test/.hive/plans/test.md",
			"RelPath": ".hive/plans/test.md",
			"Type":    "plan",
		},
	}

	if len(formFields) > 0 {
		formData := make(map[string]any, len(formFields))
		for _, field := range formFields {
			switch {
			case field.Preset != "":
				dummyItem := map[string]any{
					"ID": "test", "Name": "test", "Path": "/tmp", "Remote": "test",
				}
				if field.Multi {
					formData[field.Variable] = []map[string]any{dummyItem}
				} else {
					formData[field.Variable] = dummyItem
				}
			case field.Type == FormTypeMultiSelect:
				formData[field.Variable] = []string{"test"}
			default:
				formData[field.Variable] = "test"
			}
		}
		data["Form"] = formData
	}

	return data
}

// validationRenderer is used for template syntax checking during config validation.
// It uses placeholder values since output is discarded — only parse errors matter.
var validationRenderer = tmpl.NewValidation()

// validateTemplate checks if a template string is valid.
func validateTemplate(tmplStr string, data any) error {
	_, err := validationRenderer.Render(tmplStr, data)
	return err
}
