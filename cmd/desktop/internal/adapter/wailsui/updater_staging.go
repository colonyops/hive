package wailsui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// errUpdateReadOnlyInstall reports an install whose directory the running user
// cannot write, so the in-place swap could never succeed. Surfaced to the
// frontend by UpdaterService.InstallUpdate.
var errUpdateReadOnlyInstall = errors.New("this Hive install is not writable by the current user; update it the same way it was installed")

// updaterExecutable resolves the running binary. Overridable in tests.
var updaterExecutable = os.Executable

// staleStagingAge is how old an orphaned wails-update-* directory must be
// before a later update sweeps it. A hard kill mid-download leaves one behind;
// anything younger may belong to a second instance updating right now.
const staleStagingAge = 24 * time.Hour

// prepareUpdateStaging arranges for the wails updater to stage its download on
// the same filesystem as the binary it will replace, by pointing $TMPDIR at
// the executable's directory, and returns a restore function the caller must
// call once the download has been staged. On non-Linux platforms it is a
// no-op: the default temp directory is already co-located with the install.
//
// The redirect exists because the updater stages under os.MkdirTemp("", ...)
// — $TMPDIR, defaulting to /tmp — while its helper swaps with a bare rename
// and no cross-device fallback (updater/helper_unix.go). On every distro that
// mounts /tmp as tmpfs (Fedora, Arch, openSUSE, RHEL 9+, Debian 13+) that
// rename fails with EXDEV and the user watches the app relaunch on the old
// version. The override is scoped to the download rather than set at startup:
// $TMPDIR is inherited by the terminal and shell commands the output worker
// spawns, and those should keep the user's real temp directory outside this
// window.
//
// It returns errUpdateReadOnlyInstall when the executable's directory is not
// writable, so callers can fail before spending a download on a swap that
// cannot succeed.
func prepareUpdateStaging(goos string) (restore func(), err error) {
	if goos != "linux" {
		return func() {}, nil
	}

	exe, err := updaterExecutable()
	if err != nil {
		return nil, fmt.Errorf("resolve running executable: %w", err)
	}
	dir := filepath.Dir(exe)

	// Probe by creating the same kind of directory the updater will, rather
	// than reading mode bits: the answer has to account for ownership, group
	// membership, ACLs, and read-only mounts, and only the kernel knows all of
	// those. The probe doubles as the writability check the helper needs.
	// Any failure here — denied permission, a read-only mount, a full disk —
	// means the helper could not write the backup or the replacement either, so
	// they all collapse to the same actionable answer for the user. The cause
	// is wrapped for the log.
	probe, err := os.MkdirTemp(dir, ".hive-update-probe-*")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUpdateReadOnlyInstall, err)
	}
	_ = os.RemoveAll(probe)

	sweepStaleStaging(dir)

	prev, had := os.LookupEnv("TMPDIR")
	if err := os.Setenv("TMPDIR", dir); err != nil {
		return nil, fmt.Errorf("redirect update staging: %w", err)
	}
	return func() {
		if had {
			_ = os.Setenv("TMPDIR", prev)
			return
		}
		_ = os.Unsetenv("TMPDIR")
	}, nil
}

// sweepStaleStagingBesideExecutable sweeps the running binary's directory,
// where prepareUpdateStaging redirects staging. Called at startup as well as
// before each install, so an abandoned download does not sit beside the binary
// until the user happens to update again.
func sweepStaleStagingBesideExecutable(goos string) {
	if goos != "linux" {
		return
	}
	exe, err := updaterExecutable()
	if err != nil {
		return
	}
	sweepStaleStaging(filepath.Dir(exe))
}

// sweepStaleStaging removes staging directories a previous update abandoned.
// The updater deletes its own on a failed download and the helper deletes it
// after a successful swap, so anything left is the residue of a hard kill.
// Best-effort throughout: a directory we cannot read or remove is skipped.
func sweepStaleStaging(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-staleStagingAge)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "wails-update-") {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(dir, entry.Name()))
	}
}

// currentGOOS indirects runtime.GOOS so tests can drive InstallUpdate's Linux
// staging path from any host.
var currentGOOS = func() string { return runtime.GOOS }
