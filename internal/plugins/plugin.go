// Package plugins provides a plugin system for extending Hive with
// additional commands and status providers.
package plugins

import (
	"context"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/hay-kot/hive/internal/core/config"
	"github.com/hay-kot/hive/internal/core/session"
)

// Plugin defines the interface for Hive plugins.
type Plugin interface {
	// Name returns the plugin name (e.g., "github", "beads").
	Name() string

	// Available returns true if the plugin's dependencies are met
	// (e.g., gh CLI is installed). Called once at startup.
	Available() bool

	// Init initializes the plugin. Called once after registration
	// if the plugin is available.
	Init(ctx context.Context) error

	// Close releases plugin resources.
	Close() error

	// Commands returns commands to register in the command palette.
	// Uses Plugin<Cmd> naming convention (e.g., GithubOpenPR).
	Commands() map[string]config.UserCommand

	// StatusProvider returns the status provider for UI integration.
	// May return nil if the plugin doesn't provide status.
	StatusProvider() StatusProvider
}

// StatusProvider defines the interface for plugins that provide status
// information to display in the UI (tree view, preview header).
type StatusProvider interface {
	// RefreshStatus fetches fresh status data for a batch of sessions.
	// Uses the shared worker pool for subprocess calls.
	// Returns a map of session ID -> Status.
	RefreshStatus(ctx context.Context, sessions []*session.Session, pool *WorkerPool) (map[string]Status, error)

	// StatusCacheDuration returns how long status can be cached.
	StatusCacheDuration() time.Duration
}

// Status represents plugin status to display in the UI.
type Status struct {
	Label string         // e.g., "0/3", "PR#42", "main +2/-1"
	Icon  string         // e.g., "●", "◆", "!"
	Style lipgloss.Style // color/formatting
}
