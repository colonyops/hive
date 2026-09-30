package wailsui

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/wailsapp/wails/v3/pkg/updater"

	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

// fakeEngine is a test double for the Updater. It records call counts and
// returns a scripted release / error.
type fakeEngine struct {
	mu         sync.Mutex
	checks     int
	installs   int
	restarts   int
	rel        *updater.Release
	checkErr   error
	installErr error
	restartErr error
}

func (f *fakeEngine) Check(context.Context) (*updater.Release, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checks++
	return f.rel, f.checkErr
}

func (f *fakeEngine) DownloadAndInstall(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installs++
	return f.installErr
}

func (f *fakeEngine) Restart(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.restarts++
	return f.restartErr
}

func (f *fakeEngine) checkCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.checks
}

// silenceEmits swaps the package-level emit hook for a no-op so tests don't
// depend on a running application.
func silenceEmits(t *testing.T) {
	t.Helper()
	orig := emitUpdateAvailable
	emitUpdateAvailable = func(UpdateInfo) {}
	t.Cleanup(func() { emitUpdateAvailable = orig })
}

// acceptWriteEnabled is a settings writer stand-in for tests that only care
// about SetEnabled's ticker start/stop behavior, not persistence. It mirrors
// what app.SettingsService.SetUpdatesEnabled returns for a store with no
// process-environment override: the value handed in, echoed back.
func acceptWriteEnabled(enabled bool) (bool, error) { return enabled, nil }

// storeWriteEnabled builds a settings writer backed by a real store, the same
// shape app.SettingsService.SetUpdatesEnabled has in production, for the one
// test that checks SetEnabled's persistence rather than just its ticker
// side effect.
func storeWriteEnabled(store *settings.Store) func(bool) (bool, error) {
	return func(enabled bool) (bool, error) {
		effective, err := store.Update(func(cfg *settings.Settings) error {
			cfg.Updates.Enabled = enabled
			return nil
		})
		return effective.Updates.Enabled, err
	}
}

func TestUpdaterServiceStatusDefault(t *testing.T) {
	silenceEmits(t)
	s := NewUpdaterService("1.2.3", true, time.Hour, acceptWriteEnabled, zerolog.Nop())
	got := s.Status()
	require.False(t, got.Available)
	require.Equal(t, "1.2.3", got.CurrentVersion)
}

func TestUpdaterServiceCheckNowAvailable(t *testing.T) {
	silenceEmits(t)
	engine := &fakeEngine{rel: &updater.Release{
		Version: "1.3.0",
		Notes:   "new stuff",
	}}
	s := NewUpdaterService("1.2.3", false, time.Hour, acceptWriteEnabled, zerolog.Nop())
	s.Attach(engine)

	info, err := s.CheckNow(t.Context())
	require.NoError(t, err)
	require.True(t, info.Available)
	require.Equal(t, "1.3.0", info.LatestVersion)
	require.Equal(t, "new stuff", info.Notes)
	// Status reflects the cached result.
	require.True(t, s.Status().Available)
}

func TestUpdaterServiceCheckNowUpToDate(t *testing.T) {
	silenceEmits(t)
	engine := &fakeEngine{rel: nil}
	s := NewUpdaterService("1.2.3", false, time.Hour, acceptWriteEnabled, zerolog.Nop())
	s.Attach(engine)

	info, err := s.CheckNow(t.Context())
	require.NoError(t, err)
	require.False(t, info.Available)
	require.Equal(t, "1.2.3", info.CurrentVersion)
	// An up-to-date answer is still an answer: About distinguishes it from
	// "never checked" by the timestamp alone.
	require.NotEmpty(t, info.CheckedAt)
	require.Equal(t, info.CheckedAt, s.Status().CheckedAt)
}

func TestUpdaterServiceCheckNowError(t *testing.T) {
	silenceEmits(t)
	engine := &fakeEngine{checkErr: errors.New("boom")}
	s := NewUpdaterService("1.2.3", false, time.Hour, acceptWriteEnabled, zerolog.Nop())
	s.Attach(engine)

	_, err := s.CheckNow(t.Context())
	require.Error(t, err)
}

func TestUpdaterServiceDevGate(t *testing.T) {
	silenceEmits(t)
	t.Setenv(settings.EnvConfigDir, filepath.Join(t.TempDir(), "config"))
	store := settings.NewStore(settings.SettingsPath())
	// No engine attached => dev build.
	s := NewUpdaterService("dev", true, time.Millisecond, storeWriteEnabled(store), zerolog.Nop())

	info, err := s.CheckNow(t.Context())
	require.NoError(t, err)
	require.False(t, info.Available)

	// SetEnabled persists but starts no ticker (engine nil), so it must not
	// panic or spin.
	require.NoError(t, s.SetEnabled(true))
	got, err := settings.LoadSettings()
	require.NoError(t, err)
	require.True(t, got.Updates.Enabled)
	s.Stop() // safe no-op
}

func TestUpdaterServiceTickerLifecycle(t *testing.T) {
	silenceEmits(t)
	synctest.Test(t, func(t *testing.T) {
		t.Setenv(settings.EnvConfigDir, filepath.Join(t.TempDir(), "config"))
		engine := &fakeEngine{rel: nil}
		s := NewUpdaterService("1.2.3", false, time.Minute, acceptWriteEnabled, zerolog.Nop())
		s.Attach(engine)

		// Enabling checks immediately (initial check) then on each tick.
		require.NoError(t, s.SetEnabled(true))
		synctest.Wait()
		require.Equal(t, 1, engine.checkCount(), "initial check on enable")

		time.Sleep(time.Minute)
		synctest.Wait()
		require.Equal(t, 2, engine.checkCount(), "one check per interval")

		// Disabling stops the ticker: no further checks.
		require.NoError(t, s.SetEnabled(false))
		synctest.Wait()
		time.Sleep(3 * time.Minute)
		synctest.Wait()
		require.Equal(t, 2, engine.checkCount(), "no checks after disable")
	})
}

func TestUpdaterServiceInstallUpdate(t *testing.T) {
	silenceEmits(t)
	engine := &fakeEngine{}
	s := NewUpdaterService("1.2.3", false, time.Hour, acceptWriteEnabled, zerolog.Nop())
	s.Attach(engine)

	require.NoError(t, s.InstallUpdate(t.Context()))
	engine.mu.Lock()
	defer engine.mu.Unlock()
	require.Equal(t, 1, engine.installs)
	require.Equal(t, 1, engine.restarts)
}

func TestUpdaterServiceInstallUpdateLogsDownloadFailure(t *testing.T) {
	silenceEmits(t)
	var logs bytes.Buffer
	engine := &fakeEngine{installErr: errors.New("checksum mismatch")}
	s := NewUpdaterService("1.2.3", false, time.Hour, acceptWriteEnabled, zerolog.New(&logs))
	s.Attach(engine)
	s.available = &UpdateInfo{LatestVersion: "1.3.0"}

	err := s.InstallUpdate(t.Context())
	require.ErrorContains(t, err, "checksum mismatch")
	require.Contains(t, logs.String(), `"stage":"download_install"`)
	require.Contains(t, logs.String(), `"current_version":"1.2.3"`)
	require.Contains(t, logs.String(), `"latest_version":"1.3.0"`)
	require.Contains(t, logs.String(), `"message":"update install failed"`)
}

func TestUpdaterServiceInstallUpdateLogsRestartFailure(t *testing.T) {
	silenceEmits(t)
	var logs bytes.Buffer
	engine := &fakeEngine{restartErr: errors.New("helper failed")}
	s := NewUpdaterService("1.2.3", false, time.Hour, acceptWriteEnabled, zerolog.New(&logs))
	s.Attach(engine)

	err := s.InstallUpdate(t.Context())
	require.ErrorContains(t, err, "helper failed")
	require.Contains(t, logs.String(), `"stage":"restart"`)
	require.Contains(t, logs.String(), `"message":"update install failed"`)
}

func TestUpdaterServiceInstallUpdateDevNoop(t *testing.T) {
	silenceEmits(t)
	s := NewUpdaterService("dev", false, time.Hour, acceptWriteEnabled, zerolog.Nop())
	require.NoError(t, s.InstallUpdate(t.Context()))
}

// A read-only install can never be swapped in place, and the user should learn
// that before the download runs, not after.
func TestUpdaterServiceInstallUpdateRejectsReadOnlyInstallBeforeDownload(t *testing.T) {
	silenceEmits(t)
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions, so the probe cannot fail")
	}
	dir := t.TempDir()
	stubExecutable(t, dir)
	stubGOOS(t, "linux")
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	engine := &fakeEngine{}
	s := NewUpdaterService("1.2.3", false, time.Hour, acceptWriteEnabled, zerolog.Nop())
	s.Attach(engine)

	err := s.InstallUpdate(t.Context())
	require.ErrorIs(t, err, errUpdateReadOnlyInstall)
	engine.mu.Lock()
	defer engine.mu.Unlock()
	require.Equal(t, 0, engine.installs, "the download must not start for an install that cannot be swapped")
}

func TestUpdaterServiceInstallUpdateRestoresTMPDIROnFailure(t *testing.T) {
	silenceEmits(t)
	dir := t.TempDir()
	t.Setenv("TMPDIR", "/sentinel")
	stubExecutable(t, dir)
	stubGOOS(t, "linux")

	engine := &fakeEngine{installErr: errors.New("boom")}
	s := NewUpdaterService("1.2.3", false, time.Hour, acceptWriteEnabled, zerolog.Nop())
	s.Attach(engine)

	require.ErrorContains(t, s.InstallUpdate(t.Context()), "boom")
	require.Equal(t, "/sentinel", os.Getenv("TMPDIR"), "a failed download must still restore TMPDIR")
	engine.mu.Lock()
	defer engine.mu.Unlock()
	require.Equal(t, 0, engine.restarts)
}

// blockingEngine parks DownloadAndInstall until released, to hold InstallUpdate
// mid-flight while a second call arrives.
type blockingEngine struct {
	installs  atomic.Int32
	started   chan struct{}
	release   chan struct{}
	startOnce sync.Once
}

func (b *blockingEngine) Check(context.Context) (*updater.Release, error) { return nil, nil }

func (b *blockingEngine) DownloadAndInstall(context.Context) error {
	b.installs.Add(1)
	b.startOnce.Do(func() { close(b.started) })
	<-b.release
	return nil
}

func (b *blockingEngine) Restart(context.Context) error { return nil }

// The TMPDIR save/restore pair in prepareUpdateStaging is process-global, so a
// second InstallUpdate must not interleave with one already running.
func TestUpdaterServiceInstallUpdateIgnoresConcurrentRequests(t *testing.T) {
	silenceEmits(t)
	engine := &blockingEngine{started: make(chan struct{}), release: make(chan struct{})}
	s := NewUpdaterService("1.2.3", false, time.Hour, acceptWriteEnabled, zerolog.Nop())
	s.Attach(engine)

	done := make(chan error, 1)
	go func() { done <- s.InstallUpdate(context.Background()) }()
	<-engine.started

	require.NoError(t, s.InstallUpdate(t.Context()), "a duplicate request is ignored, not an error")
	require.Equal(t, int32(1), engine.installs.Load())

	close(engine.release)
	require.NoError(t, <-done)
}
