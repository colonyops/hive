package wailsui

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/wailsapp/wails/v3/pkg/updater"
)

// DefaultUpdateCheckInterval is how often the self-managed ticker polls for a
// newer desktop release when auto-update is enabled. The framework's
// Config.CheckInterval is fixed at Init time and has no runtime setter, so the
// service owns its own ticker to support a live enable/disable toggle.
const DefaultUpdateCheckInterval = 6 * time.Hour

// updaterEngine is the slice of *updater.Updater the service drives. Declaring
// it as an interface keeps UpdaterService unit-testable without a live,
// network-backed Updater. *updater.Updater satisfies it.
type updaterEngine interface {
	Check(ctx context.Context) (*updater.Release, error)
	DownloadAndInstall(ctx context.Context) error
	Restart(ctx context.Context) error
}

// UpdateInfo is the frontend-facing view of the last check result plus the
// current auto-update toggle state, so a single Status() call seeds both the
// title-bar chip and the settings switch.
type UpdateInfo struct {
	Enabled        bool   `json:"enabled"`
	Available      bool   `json:"available"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	// Notes is whatever the release manifest carried. The provider does not
	// populate it today; there is no public changelog to link to instead, the
	// source repository being private.
	Notes string `json:"notes"`
	// CheckedAt is when the cached result was produced, RFC3339, or empty when
	// no check has completed. It covers background poll ticks too, so the
	// About screen can say when the app last looked without the user pressing
	// anything.
	CheckedAt string `json:"checkedAt"`
}

// UpdaterService exposes desktop self-update to the frontend: an enable/disable
// toggle backed by a self-managed poll ticker, a manual check, and an install
// action. On dev/unreleased builds the engine is nil (Init is skipped so every
// real release would not register as "newer"), so the service reports
// Available:false and the ticker never runs.
type UpdaterService struct {
	currentVersion string
	interval       time.Duration
	logger         zerolog.Logger
	// writeEnabled persists the toggle through the core: the adapter has no
	// settings-mutation logic of its own (see app.SettingsService.SetUpdatesEnabled,
	// which is what every real caller passes).
	writeEnabled func(bool) (bool, error)

	mu        sync.Mutex
	engine    updaterEngine
	enabled   bool
	available *UpdateInfo
	cancel    context.CancelFunc
	wg        sync.WaitGroup

	// installMu serializes InstallUpdate: the TMPDIR save/restore pair in
	// prepareUpdateStaging is process-global and must not interleave.
	installMu sync.Mutex
}

// NewUpdaterService constructs the service. engine is attached later via
// attach once app.Updater is initialized (which can only happen after
// application.New). enabled seeds the persisted toggle state. writeEnabled is
// the core's settings mutation (app.SettingsService.SetUpdatesEnabled in
// production); the adapter never reads or writes settings.yaml itself.
func NewUpdaterService(currentVersion string, enabled bool, interval time.Duration, writeEnabled func(bool) (bool, error), logger zerolog.Logger) *UpdaterService {
	if interval <= 0 {
		interval = DefaultUpdateCheckInterval
	}
	return &UpdaterService{
		currentVersion: currentVersion,
		interval:       interval,
		enabled:        enabled,
		logger:         logger.With().Str("component", "updater").Logger(),
		writeEnabled:   writeEnabled,
	}
}

// Attach wires the live Updater engine and, when auto-update is enabled,
// starts the background poll ticker. Called from main.go after Updater.Init on
// release builds; never called on dev builds, so the engine stays nil.
//
// It had to be exported to survive the move out of package main, so it carries
// wails:ignore to keep it off the RPC surface -- its interface argument is not
// JSON-marshalable, and the frontend has no business starting the poll loop.
//
//wails:ignore
func (s *UpdaterService) Attach(engine updaterEngine) {
	// A hard-killed update leaves its staging directory beside the binary;
	// sweep at startup too, not only before the next install, so it does not
	// sit there indefinitely when the user never updates again.
	sweepStaleStagingBesideExecutable(currentGOOS())
	s.mu.Lock()
	s.engine = engine
	start := s.enabled && s.engine != nil
	if start {
		s.startLoopLocked()
	}
	s.mu.Unlock()
}

// Status returns the last cached check result for initial render. When no check
// has run yet it reports the running version with Available:false.
func (s *UpdaterService) Status() UpdateInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.available != nil {
		info := *s.available
		info.Enabled = s.enabled
		return info
	}
	return UpdateInfo{Enabled: s.enabled, Available: false, CurrentVersion: s.currentVersion}
}

// SetEnabled persists the toggle to settings and starts/stops the ticker
// atomically. On dev builds (no engine) it still persists the preference but
// starts no ticker.
func (s *UpdaterService) SetEnabled(enabled bool) error {
	effective, err := s.writeEnabled(enabled)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = effective
	s.stopLoopLocked()
	if effective && s.engine != nil {
		s.startLoopLocked()
	}
	return nil
}

// CheckNow runs a manual silent check, updates the cache, and emits
// update:available when a newer release exists. On dev builds it reports
// Available:false.
func (s *UpdaterService) CheckNow(ctx context.Context) (UpdateInfo, error) {
	return s.check(ctx)
}

// InstallUpdate downloads + verifies the pending release, then relaunches into
// it. Requires a prior successful check that found an update.
func (s *UpdaterService) InstallUpdate(ctx context.Context) error {
	s.mu.Lock()
	engine := s.engine
	available := s.available
	s.mu.Unlock()
	if engine == nil {
		s.logger.Debug().Msg("update install ignored; updater is unavailable")
		return nil
	}

	latestVersion := ""
	if available != nil {
		latestVersion = available.LatestVersion
	}
	log := s.logger.With().Str("current_version", s.currentVersion).Str("latest_version", latestVersion).Logger()
	if !s.installMu.TryLock() {
		log.Info().Msg("update install already in progress; ignoring duplicate request")
		return nil
	}
	defer s.installMu.Unlock()
	log.Info().Msg("update install started")

	// Stage the download on the binary's own filesystem and reject read-only
	// installs before downloading; no-op off Linux (see prepareUpdateStaging).
	restoreStaging, err := prepareUpdateStaging(currentGOOS())
	if err != nil {
		log.Error().Err(err).Str("stage", "prepare_staging").Msg("update install failed")
		return err
	}

	installErr := engine.DownloadAndInstall(ctx)
	// The staged artifact stays on disk for Restart to hand to the helper; only
	// the environment override is scoped to the download.
	restoreStaging()
	if installErr != nil {
		log.Error().Err(installErr).Str("stage", "download_install").Msg("update install failed")
		return installErr
	}
	log.Info().Msg("update downloaded and verified; requesting restart")

	if err := engine.Restart(ctx); err != nil {
		log.Error().Err(err).Str("stage", "restart").Msg("update install failed")
		return err
	}
	log.Info().Msg("update restart requested")
	return nil
}

// Stop cancels the ticker and waits for the poll goroutine to exit. Safe to
// call when no ticker is running. Exported for main.go's shutdown path only;
// wails:ignore keeps it off the RPC surface.
//
//wails:ignore
func (s *UpdaterService) Stop() {
	s.mu.Lock()
	s.stopLoopLocked()
	s.mu.Unlock()
	s.wg.Wait()
}

// check performs the engine check, caches the result, and emits the matching
// event. Held locks are released around the network round trip.
func (s *UpdaterService) check(ctx context.Context) (UpdateInfo, error) {
	s.mu.Lock()
	engine := s.engine
	enabled := s.enabled
	s.mu.Unlock()
	if engine == nil {
		info := UpdateInfo{Enabled: enabled, Available: false, CurrentVersion: s.currentVersion}
		return info, nil
	}

	rel, err := engine.Check(ctx)
	if err != nil {
		s.logger.Debug().Err(err).Msg("update check failed")
		return UpdateInfo{Enabled: enabled, Available: false, CurrentVersion: s.currentVersion}, err
	}

	checkedAt := time.Now().UTC().Format(time.RFC3339)
	var info UpdateInfo
	if rel == nil {
		info = UpdateInfo{Enabled: enabled, Available: false, CurrentVersion: s.currentVersion, CheckedAt: checkedAt}
	} else {
		info = UpdateInfo{
			Enabled:        enabled,
			Available:      true,
			CurrentVersion: s.currentVersion,
			LatestVersion:  rel.Version,
			Notes:          rel.Notes,
			CheckedAt:      checkedAt,
		}
	}

	s.mu.Lock()
	cached := info
	s.available = &cached
	s.mu.Unlock()

	if info.Available {
		emitUpdateAvailable(info)
	}
	return info, nil
}

// startLoopLocked launches the poll goroutine. Caller must hold s.mu and must
// have stopped any prior loop.
//
// The ticker is rooted at Background rather than a request or an app-lifetime
// context: it outlives every call that can start it, and Stop — which main's
// shutdown calls — is what ends it deterministically. Capturing an
// app-lifetime context here would mean storing one on the service for no
// added guarantee.
func (s *UpdaterService) startLoopLocked() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.wg.Add(1)
	go s.runLoop(ctx)
}

// stopLoopLocked cancels a running loop. Caller must hold s.mu.
func (s *UpdaterService) stopLoopLocked() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

// runLoop checks immediately, then on every interval tick, until ctx is
// cancelled by a disable or Stop.
func (s *UpdaterService) runLoop(ctx context.Context) {
	defer s.wg.Done()
	_, _ = s.check(ctx)
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_, _ = s.check(ctx)
		}
	}
}
