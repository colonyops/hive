package app

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/hive/internal/config"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/doctor"
	"github.com/colonyops/hive/internal/hive/events"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/pkg/executil/executiltest"
)

// The doctor check never touches tmux, so no method is ever called.
type unusedMux struct{ sessionsvc.Multiplexer }

// The engine config does not know keybindings, so `hive doctor` must validate
// with the CLI's config to keep reporting them.
func TestDoctorConfigCheckFailsOnAnInvalidKeybinding(t *testing.T) {
	t.Setenv("HIVE_DEFAULT_AGENT", "")
	cfg, err := config.Load("", t.TempDir())
	require.NoError(t, err)
	cfg.Views.Sessions.Keybindings["x"] = config.Keybinding{Help: "no command"}

	database, err := hive.OpenDB(t.Context(), t.TempDir(), cfg.Database)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	engine, err := hive.New(&cfg.Config, hive.Ports{
		DB:       database,
		Bus:      events.New(16),
		Executor: &executiltest.Exec{},
		Mux:      unusedMux{},
		DataDir:  t.TempDir(),
		Logger:   zerolog.Nop(),
	})
	require.NoError(t, err)

	app := NewApp(engine, cfg, nil, nil, nil, nil, nil)
	results := app.Doctor.RunChecks(t.Context(), "", false)

	var configResult *doctor.Result
	for i := range results {
		if results[i].Name == "Configuration" {
			configResult = &results[i]
		}
	}
	require.NotNil(t, configResult)
	require.NotEmpty(t, configResult.Items)
	assert.Equal(t, `views.sessions.keybindings["x"]`, configResult.Items[0].Label)
	assert.Equal(t, doctor.StatusFail, configResult.Items[0].Status)
	assert.Contains(t, configResult.Items[0].Detail, "cmd is required")
}
