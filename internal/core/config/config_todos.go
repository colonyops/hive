package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/hay-kot/criterio"
)

// TodosConfig holds configuration for the human todo system.
type TodosConfig struct {
	Actions       map[string]string  `yaml:"actions"` // scheme -> command template
	Limiter       TodosLimiterConfig `yaml:"limiter"`
	Notifications TodosNotifyConfig  `yaml:"notifications"`
}

// TodosLimiterConfig holds rate limiting settings for todo creation.
type TodosLimiterConfig struct {
	MaxPending          int           `yaml:"max_pending"`
	RateLimitPerSession time.Duration `yaml:"rate_limit_per_session"`
}

// TodosNotifyConfig holds notification settings for the todo system.
type TodosNotifyConfig struct {
	Toast bool `yaml:"toast"`
}

// ActionTemplateData provides template variables for custom action templates.
type ActionTemplateData struct {
	Scheme string
	Value  string
	URI    string
}

var builtinSchemes = map[string]bool{
	"session": true,
	"review":  true,
	"http":    true,
	"https":   true,
}

// validateTodos checks that the todo configuration is valid.
func (c *Config) validateTodos() error {
	var errs criterio.FieldErrorsBuilder

	if c.Todos.Limiter.MaxPending < 0 {
		errs = errs.Append("todos.limiter.max_pending", fmt.Errorf("must be >= 0"))
	}
	if c.Todos.Limiter.RateLimitPerSession < 0 {
		errs = errs.Append("todos.limiter.rate_limit_per_session", fmt.Errorf("must be >= 0"))
	}

	if len(c.Todos.Actions) == 0 {
		return errs.ToError()
	}
	for scheme, tmplStr := range c.Todos.Actions {
		field := fmt.Sprintf("todos.actions[%q]", scheme)
		normalized := strings.ToLower(scheme)
		if builtinSchemes[normalized] {
			errs = errs.Append(field, fmt.Errorf("cannot override built-in scheme %q", normalized))
			continue
		}
		if err := validateTemplate(tmplStr, ActionTemplateData{}); err != nil {
			errs = errs.Append(field, fmt.Errorf("invalid template: %w", err))
		}
	}
	return errs.ToError()
}
