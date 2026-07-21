package main

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"

	"github.com/colonyops/hive/internal/desktop"
)

// defaultUpdateCheckInterval is how often the self-managed ticker polls for a
// newer desktop release when auto-update is enabled. The framework's
// Config.CheckInterval is fixed at Init time and has no runtime setter, so the
// service owns its own ticker to support a live enable/disable toggle.
const defaultUpdateCheckInterval = 6 * time.Hour

// updateAvailableEvent is emitted when a check finds a newer release; the title
// bar subscribes to it. updateNoneEvent fires when a check confirms the app is
// up to date.
const (
	updateAvailableEvent = "update:available"
	updateNoneEvent      = "update:none"
)

// updaterEngine is the slice of *updater.Updater the service drives. Declaring
// it as an interface keeps UpdaterService unit-testable without a live,
// network-backed Updater. *updater.Updater satisfies it.
type updaterEngine interface {
	Check(ctx context.Context) (*updater.Release, error)
	DownloadAndInstall(ctx context.Context) error
	Restart(ctx context.Context) error
}

// UpdateInfo is the frontend-facing view of the last check result.
type UpdateInfo struct {
	Available      bool   `json:"available"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	Notes          string `json:"notes"`
	ReleaseURL     string `json:"releaseUrl"`
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

	mu        sync.Mutex
	engine    updaterEngine
	enabled   bool
	available *UpdateInfo
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// NewUpdaterService constructs the service. engine is attached later via
// attach once app.Updater is initialized (which can only happen after
// application.New). enabled seeds the persisted toggle state.
func NewUpdaterService(currentVersion string, enabled bool, interval time.Duration, logger zerolog.Logger) *UpdaterService {
	if interval <= 0 {
		interval = defaultUpdateCheckInterval
	}
	return &UpdaterService{
		currentVersion: currentVersion,
		interval:       interval,
		enabled:        enabled,
		logger:         logger.With().Str("component", "updater").Logger(),
	}
}

// attach wires the live Updater engine and, when auto-update is enabled, starts
// the background poll ticker. Called from main.go after Updater.Init on release
// builds; never called on dev builds, so the engine stays nil. Unexported so
// Wails does not surface it as a frontend RPC (its interface arg is not
// JSON-marshalable anyway).
func (s *UpdaterService) attach(engine updaterEngine) {
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
		return *s.available
	}
	return UpdateInfo{Available: false, CurrentVersion: s.currentVersion}
}

// SetEnabled persists the toggle to settings and starts/stops the ticker
// atomically. On dev builds (no engine) it still persists the preference but
// starts no ticker.
func (s *UpdaterService) SetEnabled(enabled bool) error {
	current, err := desktop.LoadSettings()
	if err != nil {
		return err
	}
	current.AutoUpdate = &enabled
	if err := desktop.SaveSettings(current); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = enabled
	s.stopLoopLocked()
	if enabled && s.engine != nil {
		s.startLoopLocked()
	}
	return nil
}

// CheckNow runs a manual silent check, updates the cache, and emits
// update:available / update:none. On dev builds it reports Available:false.
func (s *UpdaterService) CheckNow() (UpdateInfo, error) {
	return s.check(context.Background())
}

// InstallUpdate downloads + verifies the pending release, then relaunches into
// it. Requires a prior successful check that found an update.
func (s *UpdaterService) InstallUpdate() error {
	s.mu.Lock()
	engine := s.engine
	s.mu.Unlock()
	if engine == nil {
		return nil
	}
	ctx := context.Background()
	if err := engine.DownloadAndInstall(ctx); err != nil {
		return err
	}
	return engine.Restart(ctx)
}

// stop cancels the ticker and waits for the poll goroutine to exit. Safe to
// call when no ticker is running.
func (s *UpdaterService) stop() {
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
	s.mu.Unlock()
	if engine == nil {
		info := UpdateInfo{Available: false, CurrentVersion: s.currentVersion}
		return info, nil
	}

	rel, err := engine.Check(ctx)
	if err != nil {
		s.logger.Debug().Err(err).Msg("update check failed")
		return UpdateInfo{Available: false, CurrentVersion: s.currentVersion}, err
	}

	var info UpdateInfo
	if rel == nil {
		info = UpdateInfo{Available: false, CurrentVersion: s.currentVersion}
	} else {
		info = UpdateInfo{
			Available:      true,
			CurrentVersion: s.currentVersion,
			LatestVersion:  rel.Version,
			Notes:          rel.Notes,
			ReleaseURL:     releaseURLFromMetadata(rel),
		}
	}

	s.mu.Lock()
	cached := info
	s.available = &cached
	s.mu.Unlock()

	if info.Available {
		emitUpdateAvailable(info)
	} else {
		emitUpdateNone(info)
	}
	return info, nil
}

// startLoopLocked launches the poll goroutine. Caller must hold s.mu and must
// have stopped any prior loop.
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

// releaseURLFromMetadata prefers the release HTML URL the provider stashed on
// the Release, falling back to a computed desktop release URL.
func releaseURLFromMetadata(rel *updater.Release) string {
	if rel.Metadata != nil {
		if u, ok := rel.Metadata["github.release.url"].(string); ok && u != "" {
			return u
		}
	}
	return releaseURL(rel.Version)
}

// --- event + settings indirection (overridable in tests) ---

var (
	emitUpdateAvailable = func(info UpdateInfo) {
		if app := application.Get(); app != nil {
			app.Event.Emit(updateAvailableEvent, info)
		}
	}
	emitUpdateNone = func(info UpdateInfo) {
		if app := application.Get(); app != nil {
			app.Event.Emit(updateNoneEvent, info)
		}
	}
)
