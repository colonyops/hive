package config

import (
	"fmt"
	"os"
	"reflect"
	"strings"
)

// LoadReload reads a complete replacement, retaining the running configuration
// if a startup resource would need to be rebuilt.
func LoadReload(path string, current *Config) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, fmt.Errorf("config %q is empty", path)
	}
	cfg, err := loadBytes(data, current.DataDir)
	if err != nil {
		return nil, fmt.Errorf("load config %q: %w", path, err)
	}
	if err := cfg.ValidateDeep(path); err != nil {
		return nil, fmt.Errorf("validate config %q: %w", path, err)
	}
	var changed []string
	for _, field := range []struct {
		name          string
		before, after any
	}{
		{"database", current.Database, cfg.Database},
		{"plugins", current.Plugins, cfg.Plugins},
		{"context", current.Context, cfg.Context},
		{"workspaces", current.Workspaces, cfg.Workspaces},
		{"tui.update_checker", current.TUI.UpdateChecker, cfg.TUI.UpdateChecker},
		{"todos.notifications.toast", current.Todos.Notifications.Toast, cfg.Todos.Notifications.Toast},
	} {
		if !reflect.DeepEqual(field.before, field.after) {
			changed = append(changed, field.name)
		}
	}
	if len(changed) > 0 {
		return nil, fmt.Errorf("restart required for %s; no configuration changes applied", strings.Join(changed, ", "))
	}
	return cfg, nil
}
