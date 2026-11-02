// Package apptest builds an app.Config whose whole world sits in one test's temp
// directory, without touching the process environment. A test using it can run
// in parallel, and nothing it does reaches a developer's own config or data.
package apptest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/platform/execenv"
)

// Config returns the Config for a mock-mode App.
func Config(t *testing.T) app.Config {
	t.Helper()
	root := t.TempDir()
	return app.Config{
		Settings: settings.DefaultSettings(),
		Paths: settings.ResolvePaths(
			settings.Bootstrap{DataDir: filepath.Join(root, "data"), ConfigDir: filepath.Join(root, "config")},
			settings.ResolveOptions{Env: func(string) string { return "" }},
		),
		MockMode: settings.MockFeed,
		Logger:   zerolog.Nop(),
		ExecEnv: execenv.NewResolver(execenv.Options{
			Pinned: map[string]string{config.EnvConfig: filepath.Join(root, "hive.yaml")},
			// The real probe sources the developer's shell startup files.
			Probe: func(context.Context, string) (map[string]string, error) {
				return map[string]string{"PATH": os.Getenv("PATH")}, nil
			},
		}),
	}
}
