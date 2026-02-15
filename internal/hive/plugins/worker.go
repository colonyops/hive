package plugins

import "github.com/colonyops/hive/internal/core/session"

// Job represents a single status fetch request for a plugin and session.
type Job struct {
	PluginName string
	SessionID  string
	Session    *session.Session
}

// Result represents the outcome of a status fetch job.
type Result struct {
	PluginName string
	SessionID  string
	Status     Status
	Err        error
}
