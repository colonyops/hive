package config

import "strings"

const EnvAnalyticsEnabled = "HIVE_ANALYTICS_ENABLED"

// AnalyticsConfig uses pointers so omission remains distinct from explicit false.
type AnalyticsConfig struct {
	Enabled *bool                `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Local   AnalyticsLocalConfig `json:"local"             yaml:"local"`
}

type AnalyticsLocalConfig struct {
	Enabled *bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`
}

func (c AnalyticsConfig) CollectionEnabled() bool {
	return (c.Enabled == nil || *c.Enabled) && (c.Local.Enabled == nil || *c.Local.Enabled)
}

// AnalyticsEnvironmentDisabled is a kill switch, never a force-enable override.
func AnalyticsEnvironmentDisabled(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "false") || strings.TrimSpace(value) == "0"
}
