package config

import "time"

// TodosConfig holds configuration for the todo system.
type TodosConfig struct {
	Actions       map[string]string  `yaml:"actions"`
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
