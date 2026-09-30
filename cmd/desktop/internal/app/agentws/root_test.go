package agentws

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureRoot(t *testing.T) {
	t.Run("CreatesADefaultRoot", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "workspaces")

		created, err := EnsureRoot(root)
		require.NoError(t, err)
		assert.True(t, created)
		info, statErr := os.Stat(root)
		require.NoError(t, statErr)
		assert.True(t, info.IsDir())
	})

	t.Run("SecondCallReportsAlreadyPresent", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "workspaces")
		_, err := EnsureRoot(root)
		require.NoError(t, err)

		created, err := EnsureRoot(root)
		require.NoError(t, err)
		assert.False(t, created)
	})

	t.Run("MissingParentIsUnavailable", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "no-such-parent", "workspaces")

		created, err := EnsureRoot(root)
		assert.False(t, created)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrRootUnavailable)
		_, statErr := os.Stat(root)
		assert.True(t, os.IsNotExist(statErr), "an unavailable root must create nothing")
	})

	t.Run("RegularFileIsNotADirectory", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "workspaces")
		require.NoError(t, os.WriteFile(root, []byte("x"), 0o600))

		created, err := EnsureRoot(root)
		assert.False(t, created)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrRootNotADirectory)
	})

	// Precedent: TestSeedDefaultsIfMissingReportsDirectoryFailure
	// (internal/app/actions/seed_test.go) — a read-only parent must surface as
	// a write failure, not be silently swallowed into a false "success".
	t.Run("ReadOnlyParentReportsTheWriteError", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("running as root: chmod 0o500 does not block writes")
		}
		parent := t.TempDir()
		root := filepath.Join(parent, "workspaces")
		require.NoError(t, os.Chmod(parent, 0o500))
		t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })

		created, err := EnsureRoot(root)
		assert.False(t, created)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrRootUnavailable, "the parent exists; this is a write failure, not an unavailable root")
		require.NotErrorIs(t, err, ErrRootNotADirectory)
		_, statErr := os.Stat(root)
		assert.True(t, os.IsNotExist(statErr), "a failed create must leave nothing behind")
	})
}
