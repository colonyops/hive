package actions

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedDefaultsIfMissingInstallsExactBytesOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "actions.yml")
	seeded, err := SeedDefaultsIfMissing(path)
	require.NoError(t, err)
	assert.True(t, seeded)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	if string(got) != defaultActionsYAML {
		t.Fatal("seed bytes differ from exact default catalog")
	}

	seeded, err = SeedDefaultsIfMissing(path)
	require.NoError(t, err)
	assert.False(t, seeded)
	got, err = os.ReadFile(path)
	require.NoError(t, err)
	if string(got) != defaultActionsYAML {
		t.Fatal("second seed altered default catalog bytes")
	}
}

func TestSeedDefaultsIfMissingNeverTouchesPresentFiles(t *testing.T) {
	for _, original := range []string{"", "version: nope\nactions: ["} {
		t.Run("present", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "actions.yml")
			require.NoError(t, os.WriteFile(path, []byte(original), 0o600))
			seeded, err := SeedDefaultsIfMissing(path)
			require.NoError(t, err)
			assert.False(t, seeded)
			got, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, original, string(got))
		})
	}
}

func TestSeedDefaultsIfMissingExclusiveConcurrentInstallAndCleansTemps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "actions.yml")
	var wg sync.WaitGroup
	results := make(chan bool, 8)
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			seeded, err := SeedDefaultsIfMissing(path)
			results <- seeded
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	count := 0
	for seeded := range results {
		if seeded {
			count++
		}
	}
	for err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(t, 1, count)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"actions.yml"}, func() []string {
		out := make([]string, len(entries))
		for i, entry := range entries {
			out[i] = entry.Name()
		}
		return out
	}())
}

func TestSeedDefaultsIfMissingReportsDirectoryFailure(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(parent, []byte("x"), 0o600))
	seeded, err := SeedDefaultsIfMissing(filepath.Join(parent, "actions.yml"))
	assert.False(t, seeded)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stat actions seed target")
}
