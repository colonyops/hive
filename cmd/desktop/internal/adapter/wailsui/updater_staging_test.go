package wailsui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// stubExecutable points updaterExecutable at a binary inside dir for the test's
// lifetime.
func stubExecutable(t *testing.T, dir string) {
	t.Helper()
	exe := filepath.Join(dir, "hive-desktop")
	require.NoError(t, os.WriteFile(exe, []byte("binary"), 0o755))
	original := updaterExecutable
	updaterExecutable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { updaterExecutable = original })
}

// stubGOOS fixes currentGOOS for the test's lifetime so the Linux staging path
// runs on any host.
func stubGOOS(t *testing.T, goos string) {
	t.Helper()
	original := currentGOOS
	currentGOOS = func() string { return goos }
	t.Cleanup(func() { currentGOOS = original })
}

func TestPrepareUpdateStagingNonLinuxIsNoop(t *testing.T) {
	// t.TempDir() itself honours TMPDIR, so allocate before overriding it.
	dir := t.TempDir()
	t.Setenv("TMPDIR", "/sentinel")
	stubExecutable(t, dir)

	restore, err := prepareUpdateStaging("darwin")
	require.NoError(t, err)
	require.Equal(t, "/sentinel", os.Getenv("TMPDIR"), "non-Linux platforms must not touch TMPDIR")
	restore()
	require.Equal(t, "/sentinel", os.Getenv("TMPDIR"))
}

// The whole point of the redirect: the staging directory has to be created on
// the filesystem holding the binary, because the helper's swap is a bare
// rename with no cross-device fallback.
func TestPrepareUpdateStagingPointsTMPDIRAtExecutableDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", "/sentinel")
	stubExecutable(t, dir)

	restore, err := prepareUpdateStaging("linux")
	require.NoError(t, err)
	require.Equal(t, dir, os.Getenv("TMPDIR"))

	// os.MkdirTemp("", ...) is what the wails updater calls; it must now land
	// beside the binary rather than in /tmp. t.TempDir() would defeat the test:
	// the point is that this exact call honours the redirected TMPDIR.
	staging, err := os.MkdirTemp("", "wails-update-*") //nolint:usetesting // verifying the updater's own TMPDIR-honouring call, not allocating a scratch dir
	require.NoError(t, err)
	require.Equal(t, dir, filepath.Dir(staging))

	restore()
	require.Equal(t, "/sentinel", os.Getenv("TMPDIR"), "restore must put the user's TMPDIR back")
}

func TestPrepareUpdateStagingRestoresUnsetTMPDIR(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", "")
	require.NoError(t, os.Unsetenv("TMPDIR"))
	stubExecutable(t, dir)

	restore, err := prepareUpdateStaging("linux")
	require.NoError(t, err)
	require.Equal(t, dir, os.Getenv("TMPDIR"))

	restore()
	_, present := os.LookupEnv("TMPDIR")
	require.False(t, present, "TMPDIR was unset before; restore must leave it unset")
}

// A tarball unpacked into a root-owned prefix cannot be swapped in place, and
// the user should learn that before a download runs rather than after.
func TestPrepareUpdateStagingRejectsUnwritableInstall(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions, so the probe cannot fail")
	}
	dir := t.TempDir()
	stubExecutable(t, dir)
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	restore, err := prepareUpdateStaging("linux")
	require.Nil(t, restore)
	require.ErrorIs(t, err, errUpdateReadOnlyInstall)
}

func TestSweepStaleStagingBesideExecutable(t *testing.T) {
	dir := t.TempDir()
	stubExecutable(t, dir)
	stale := filepath.Join(dir, "wails-update-old")
	require.NoError(t, os.Mkdir(stale, 0o755))
	old := time.Now().Add(-staleStagingAge - time.Hour)
	require.NoError(t, os.Chtimes(stale, old, old))

	sweepStaleStagingBesideExecutable("darwin")
	require.DirExists(t, stale, "non-Linux platforms never redirect staging, so there is nothing to sweep")

	sweepStaleStagingBesideExecutable("linux")
	require.NoDirExists(t, stale)
}

func TestSweepStaleStaging(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "wails-update-old")
	fresh := filepath.Join(dir, "wails-update-new")
	unrelated := filepath.Join(dir, "some-other-dir")
	for _, d := range []string{stale, fresh, unrelated} {
		require.NoError(t, os.Mkdir(d, 0o755))
	}
	old := time.Now().Add(-staleStagingAge - time.Hour)
	require.NoError(t, os.Chtimes(stale, old, old))
	require.NoError(t, os.Chtimes(unrelated, old, old))

	sweepStaleStaging(dir)

	require.NoDirExists(t, stale, "an abandoned staging dir past the cutoff should be swept")
	require.DirExists(t, fresh, "a recent staging dir may belong to another instance updating now")
	require.DirExists(t, unrelated, "only wails-update-* directories are ours to remove")
}

func TestPrepareUpdateStagingSurfacesExecutableResolutionFailure(t *testing.T) {
	original := updaterExecutable
	updaterExecutable = func() (string, error) { return "", errors.New("boom") }
	t.Cleanup(func() { updaterExecutable = original })

	restore, err := prepareUpdateStaging("linux")
	require.Nil(t, restore)
	require.ErrorContains(t, err, "boom")
}
