package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

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

var sourceViewScopePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// Validate checks that a saved source view has valid name, base, query, and
// optional scope fields.
func (v SourceViewConfig) Validate() error {
	if strings.TrimSpace(v.Name) == "" {
		return fmt.Errorf("source view %q name: must not be empty", v.Name)
	}
	if !isValidCommandName(v.Name) {
		return fmt.Errorf("source view %q name: must contain only alphanumeric characters, dashes, and underscores", v.Name)
	}
	if v.Name == "issues" || v.Name == "prs" {
		return fmt.Errorf("source view %q name: conflicts with built-in source %q", v.Name, v.Name)
	}
	if v.Base != "issues" && v.Base != "prs" {
		return fmt.Errorf("source view %q base: must be either %q or %q", v.Name, "issues", "prs")
	}
	if strings.TrimSpace(v.Query) == "" {
		return fmt.Errorf("source view %q query: must not be empty", v.Name)
	}
	if err := rejectControlChars("query", v.Query); err != nil {
		return fmt.Errorf("source view %q: %w", v.Name, err)
	}
	if v.Scope == "" {
		return nil
	}
	if err := rejectControlChars("scope", v.Scope); err != nil {
		return fmt.Errorf("source view %q: %w", v.Name, err)
	}
	if !sourceViewScopePattern.MatchString(v.Scope) {
		return fmt.Errorf("source view %q scope: must have owner/repo format", v.Name)
	}
	return nil
}

// rejectControlChars rejects strings containing control characters so source
// view values remain safe to pass as command arguments.
func rejectControlChars(field, value string) error {
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("%s contains control character %U", field, r)
		}
	}
	return nil
}

// validateSources checks the top-level sources config template syntax, host
// overrides, and saved views.
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

	viewNames := make(map[string]int, len(c.Sources.Views))
	for i, view := range c.Sources.Views {
		field := fmt.Sprintf("sources.views[%d]", i)
		if err := view.Validate(); err != nil {
			errs = errs.Append(field, err)
		}
		if first, duplicate := viewNames[view.Name]; duplicate {
			errs = errs.Append(field+".name", fmt.Errorf("duplicate source view name %q (first declared at sources.views[%d])", view.Name, first))
		} else {
			viewNames[view.Name] = i
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

	warnings = append(warnings, c.viewCommandWarnings()...)
	warnings = append(warnings, c.sourceViewWarnings()...)
	return warnings
}

// viewCommandWarnings reports source view command collisions in declaration
// order, matching SystemCommands and CommandSet precedence.
func (c *Config) viewCommandWarnings() []ValidationWarning {
	warnings := make([]ValidationWarning, 0)
	generated := make(map[string]string, len(c.Sources.Views))

	for _, view := range c.Sources.Views {
		commandName := normalizeViewCommandName(view.Name)
		if _, collides := defaultUserCommands[commandName]; collides {
			warnings = append(warnings, ValidationWarning{
				Category: "Sources",
				Item:     view.Name,
				Message:  fmt.Sprintf("generated command %q for source view %q conflicts with a built-in command; built-in command wins", commandName, view.Name),
			})
			continue
		}
		if firstView, collides := generated[commandName]; collides {
			warnings = append(warnings, ValidationWarning{
				Category: "Sources",
				Item:     view.Name,
				Message:  fmt.Sprintf("generated command %q for source view %q conflicts with source view %q; first declared view %q wins", commandName, view.Name, firstView, firstView),
			})
			continue
		}

		generated[commandName] = view.Name
		if _, collides := c.UserCommands[commandName]; collides {
			warnings = append(warnings, ValidationWarning{
				Category: "Sources",
				Item:     view.Name,
				Message:  fmt.Sprintf("generated command %q for source view %q conflicts with a user command; user command wins", commandName, view.Name),
			})
		}
	}

	return warnings
}

// sourceViewWarnings reports configured views that cannot run because gh is
// unavailable without making configuration validation fail.
func (c *Config) sourceViewWarnings() []ValidationWarning {
	if len(c.Sources.Views) == 0 {
		return nil
	}
	if _, err := exec.LookPath("gh"); err == nil {
		return nil
	}

	names := make([]string, 0, len(c.Sources.Views))
	for _, view := range c.Sources.Views {
		names = append(names, view.Name)
	}
	return []ValidationWarning{{
		Category: "Sources",
		Item:     "views",
		Message:  fmt.Sprintf("gh executable not found on PATH; affected source views: %s", strings.Join(names, ", ")),
	}}
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
