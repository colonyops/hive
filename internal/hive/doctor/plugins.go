package doctor

import "context"

// PluginInfo describes a plugin's availability for doctor checks.
// Decoupled from the plugin interface to avoid import cycles.
type PluginInfo struct {
	Name      string
	Available bool
	Disabled  bool // explicitly disabled via config (enabled: false)
}

// PluginCheck reports each plugin's availability.
type PluginCheck struct {
	plugins []PluginInfo
}

// NewPluginCheck creates a new plugin availability check.
func NewPluginCheck(plugins []PluginInfo) *PluginCheck {
	return &PluginCheck{plugins: plugins}
}

func (c *PluginCheck) Name() string {
	return "Plugins"
}

func (c *PluginCheck) Run(_ context.Context) Result {
	result := Result{Name: c.Name()}

	if len(c.plugins) == 0 {
		result.Items = append(result.Items, CheckItem{
			Label:  "No plugins",
			Status: StatusPass,
			Detail: "no plugins configured",
		})
		return result
	}

	for _, p := range c.plugins {
		switch {
		case p.Available:
			result.Items = append(result.Items, CheckItem{
				Label:  p.Name,
				Status: StatusPass,
			})
		case p.Disabled:
			result.Items = append(result.Items, CheckItem{
				Label:  p.Name,
				Status: StatusPass,
				Detail: "disabled",
			})
		default:
			result.Items = append(result.Items, CheckItem{
				Label:  p.Name,
				Status: StatusWarn,
				Detail: "not available (missing dependencies)",
			})
		}
	}

	return result
}
