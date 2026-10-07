package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

func TestRetentionPolicyReadsActionRunsFromSettingsOnEveryPass(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.yaml")
	a := &App{settingsStore: settings.NewStore(path), logger: zerolog.Nop()}

	assert.Equal(t, int64(queries.DefaultActionRunLimit), a.retentionPolicy().ActionRunLimit, "a missing file keeps the default")

	require.NoError(t, os.WriteFile(path, []byte("retention:\n  action_runs: 7\n"), 0o600))
	assert.Equal(t, int64(7), a.retentionPolicy().ActionRunLimit)

	require.NoError(t, os.WriteFile(path, []byte("retention:\n  action_runs: -1\n"), 0o600))
	assert.Equal(t, int64(queries.DefaultActionRunLimit), a.retentionPolicy().ActionRunLimit, "an invalid file keeps the default")
}
