package config

import (
	"fmt"
	"time"

	"github.com/hay-kot/criterio"
)

// TodosConfig holds configuration for the human todo system.
type TodosConfig struct {
	Mode          string             `yaml:"mode"`          // "internal" (default) or "export-only"
	Limiter       TodosLimiterConfig `yaml:"limiter"`
	Export        TodosExportConfig  `yaml:"export"`
	Notifications TodosNotifyConfig  `yaml:"notifications"`
}

// TodosLimiterConfig holds rate limiting settings for todo creation.
type TodosLimiterConfig struct {
	MaxPending          int           `yaml:"max_pending"`            // default: 100
	RateLimitPerSession time.Duration `yaml:"rate_limit_per_session"` // default: 15s
}

// TodosExportConfig holds markdown export settings.
type TodosExportConfig struct {
	Enabled  bool               `yaml:"enabled"`
	Path     string             `yaml:"path"`     // file path for markdown export
	Markers  TodosExportMarkers `yaml:"markers"`  // optional bounded section markers
	Template string             `yaml:"template"` // optional Go template file path
}

// TodosExportMarkers defines the start and end markers for bounded section replacement.
type TodosExportMarkers struct {
	Start string `yaml:"start"` // e.g., "<!-- hive:todos:start -->"
	End   string `yaml:"end"`   // e.g., "<!-- hive:todos:end -->"
}

// TodosNotifyConfig holds notification settings for the todo system.
type TodosNotifyConfig struct {
	Toast bool `yaml:"toast"` // default: true
}

// validateTodos checks that the todo configuration is valid.
func (c *Config) validateTodos() error {
	return criterio.ValidateStruct(
		criterio.Run("todos.mode", c.Todos.Mode, criterio.StrOneOf("internal", "export-only")),
		c.validateTodosExport(),
	)
}

func (c *Config) validateTodosExport() error {
	if c.Todos.Mode == "export-only" && !c.Todos.Export.Enabled {
		return criterio.NewFieldErrors("todos.mode", fmt.Errorf("export-only mode requires export to be enabled"))
	}
	if !c.Todos.Export.Enabled {
		return nil
	}
	return criterio.Run("todos.export.path", c.Todos.Export.Path, criterio.Required[string])
}
