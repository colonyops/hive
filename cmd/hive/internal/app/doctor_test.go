package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/hive/internal/config"
	"github.com/colonyops/hive/internal/core/doctor"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	"github.com/colonyops/hive/internal/hive"
)

// The engine config does not know keybindings, so `hive doctor` must validate
// with the CLI's config to keep reporting them.
func TestDoctorConfigCheckFailsOnAnInvalidKeybinding(t *testing.T) {
	t.Setenv("HIVE_DEFAULT_AGENT", "")
	cfg, err := config.Load("", t.TempDir())
	require.NoError(t, err)
	cfg.Views.Sessions.Keybindings["x"] = config.Keybinding{Help: "no command"}

	database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	svc := hive.NewDoctorService(stores.NewSessionStore(database), &cfg.Config, cfg, nil)
	results := svc.RunChecks(t.Context(), "", false)

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
