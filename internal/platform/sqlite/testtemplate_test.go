package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestTemplateMigratesOnceAndWritesIndependentCopies(t *testing.T) {
	migrations := 0
	template := TestTemplate{Filename: "test.db", Migrate: func(_ context.Context, dir string) error {
		migrations++
		return os.WriteFile(filepath.Join(dir, "test.db"), []byte("migrated"), 0o600)
	}}
	first, second := t.TempDir(), t.TempDir()

	require.NoError(t, template.WriteTo(t.Context(), first))
	require.NoError(t, os.WriteFile(filepath.Join(first, "test.db"), []byte("changed"), 0o600))
	require.NoError(t, template.WriteTo(t.Context(), second))

	assert.Equal(t, 1, migrations)
	contents, err := os.ReadFile(filepath.Join(second, "test.db"))
	require.NoError(t, err)
	assert.Equal(t, "migrated", string(contents))
}

func TestTestTemplateReportsAMigrationFailureToEveryCaller(t *testing.T) {
	template := TestTemplate{Filename: "test.db", Migrate: func(context.Context, string) error { return os.ErrPermission }}

	require.ErrorIs(t, template.WriteTo(t.Context(), t.TempDir()), os.ErrPermission)
	require.ErrorIs(t, template.WriteTo(t.Context(), t.TempDir()), os.ErrPermission)
}
