package app

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/ingest"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	ghsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/github"
	ghclient "github.com/colonyops/hive/cmd/desktop/internal/app/sources/github/ghclient"
	"github.com/colonyops/hive/internal/platform/credentials"
)

// TestNewSettingsServiceReadsNotifications proves the adapter-facing
// constructor is a real settings-only view, not a stub: it reads the same
// settings.yaml a full SettingsService would, which is what lets
// wailsui.NotificationGate resolve policy through the core before app.New
// has built core.Settings itself.
func TestNewSettingsServiceReadsNotifications(t *testing.T) {
	t.Setenv(settings.EnvConfigDir, filepath.Join(t.TempDir(), "config"))
	cfg := settings.DefaultSettings()
	cfg.Notifications.Enabled = false
	cfg.Notifications.Delivery = settings.DeliverySystem
	require.NoError(t, settings.SaveSettings(cfg))

	service := NewSettingsService(settings.NewStore(settings.SettingsPath()))

	got, err := service.Notifications(t.Context())
	require.NoError(t, err)
	require.Equal(t, NotificationSettings{Enabled: false, Delivery: settings.DeliverySystem, Sound: true}, got)
}

func TestSettingsServiceSetGithubSettingsRejectsBelowFloor(t *testing.T) {
	t.Setenv(settings.EnvConfigDir, filepath.Join(t.TempDir(), "config"))
	service := newSettingsService(SettingsDeps{Store: settings.NewStore(settings.SettingsPath())})

	err := service.SetGithub(t.Context(), GithubSettings{PollInterval: settings.MinPollInterval - time.Second})
	require.Error(t, err)
	require.Equal(t, KindInvalid, KindOf(err), "a caller below the floor is asking for something invalid, not hitting a fault")
}

func TestSettingsServiceNotificationSettings(t *testing.T) {
	t.Setenv(settings.EnvConfigDir, filepath.Join(t.TempDir(), "config"))
	service := newSettingsService(SettingsDeps{Store: settings.NewStore(settings.SettingsPath())})

	got, err := service.Notifications(t.Context())
	require.NoError(t, err)
	require.Equal(t, NotificationSettings{Enabled: true, Delivery: settings.DeliveryAuto, Sound: true}, got)
}

func TestSettingsServiceSetNotificationSettingsHealsUnknownDelivery(t *testing.T) {
	t.Setenv(settings.EnvConfigDir, filepath.Join(t.TempDir(), "config"))
	service := newSettingsService(SettingsDeps{Store: settings.NewStore(settings.SettingsPath())})

	require.NoError(t, service.SetNotifications(t.Context(), NotificationSettings{
		Enabled: true, Delivery: "banner", Sound: true,
	}))

	got, err := settings.LoadSettings()
	require.NoError(t, err)
	require.Equal(t, settings.DeliveryAuto, got.Notifications.Delivery)
}

func TestSettingsServiceSetNotificationSettingsPreservesUnrelatedFields(t *testing.T) {
	t.Setenv(settings.EnvConfigDir, filepath.Join(t.TempDir(), "config"))
	cfg := settings.DefaultSettings()
	cfg.Updates.Enabled = false
	require.NoError(t, settings.SaveSettings(cfg))

	service := newSettingsService(SettingsDeps{Store: settings.NewStore(settings.SettingsPath())})
	want := NotificationSettings{Enabled: false, Delivery: settings.DeliveryApp, Sound: false}
	require.NoError(t, service.SetNotifications(t.Context(), want))

	got, err := settings.LoadSettings()
	require.NoError(t, err)
	require.Equal(t, 5*time.Minute, got.Polling.Interval.Duration())
	require.False(t, got.Updates.Enabled)
	require.False(t, got.Notifications.Enabled)
	require.Equal(t, settings.DeliveryApp, got.Notifications.Delivery)
	require.False(t, got.Notifications.Sound)
}

func TestSettingsServiceSetGithubSettingsPreservesAutoUpdate(t *testing.T) {
	t.Setenv(settings.EnvConfigDir, filepath.Join(t.TempDir(), "config"))
	// Seed an explicit updates.enabled:false alongside a poll interval.
	cfg := settings.DefaultSettings()
	cfg.Updates.Enabled = false
	require.NoError(t, settings.SaveSettings(cfg))

	service := newSettingsService(SettingsDeps{Store: settings.NewStore(settings.SettingsPath())})
	require.NoError(t, service.SetGithub(t.Context(), GithubSettings{PollInterval: 2 * time.Minute}))

	got, err := settings.LoadSettings()
	require.NoError(t, err)
	require.Equal(t, 2*time.Minute, got.Polling.Interval.Duration())
	require.False(t, got.Updates.Enabled, "updates.enabled must survive a poll-interval save")
}

// settingsServiceSources is the producer's Sources seam: one pull instance
// over the counting source below, so the test can assert a re-tick happened
// without standing up the connector registry.
type settingsServiceSources struct{ source connector.PullSource }

func (s settingsServiceSources) PullInstances(context.Context) []connector.Instance {
	return []connector.Instance{{
		Type:     "sources.test",
		Node:     connector.Node{FlowID: "profile", NodeID: "github"},
		Metadata: connector.Metadata{ProfileID: "profile", SourceKind: "generic"},
		Pull:     s.source,
	}}
}

func (settingsServiceSources) Prefetch(context.Context, []connector.Instance) error { return nil }

type settingsServiceSource struct {
	mu    sync.Mutex
	calls int
}

func (s *settingsServiceSource) Produce(_ context.Context, _ func(ingest.Msg) error) error {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	return nil
}

func (s *settingsServiceSource) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func TestSettingsServiceSetGithubSettingsPersistsAndApplies(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		t.Setenv(settings.EnvConfigDir, filepath.Join(t.TempDir(), "config"))
		fetchers := ghsource.NewFetchers(ghclient.NewClient(), credentials.NewMemoryStore(), zerolog.Nop())
		db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })
		source := &settingsServiceSource{}
		st := stores.New(db, stores.Options{})
		producer := ingest.NewProducer(ingest.ProducerDeps{
			Ingester:  st.InboxItems,
			Snapshots: st.EventLog,
			Heads:     st.SourceHeads,
			Sources:   settingsServiceSources{source},
			Interval:  time.Hour,
			Logger:    zerolog.Nop(),
		})
		service := newSettingsService(SettingsDeps{Store: settings.NewStore(settings.SettingsPath()), Producer: producer, Fetchers: fetchers})

		require.NoError(t, service.SetGithub(t.Context(), GithubSettings{PollInterval: 2 * time.Minute}))
		saved, err := settings.LoadSettings()
		require.NoError(t, err)
		require.Equal(t, 2*time.Minute, saved.Polling.Interval.Duration())

		got, err := service.Github(t.Context())
		require.NoError(t, err)
		require.Equal(t, 2*time.Minute, got.PollInterval)
		require.Equal(t, settings.MinPollInterval, got.MinPollInterval)

		producer.Start(t.Context())
		time.Sleep(2 * time.Minute)
		synctest.Wait()
		producer.Stop()
		require.Equal(t, 1, source.callCount(), "saved settings reset the live producer cadence")
	})
}

func TestSettingsServiceAppearanceSettingsDefaultsToUnset(t *testing.T) {
	t.Setenv(settings.EnvConfigDir, filepath.Join(t.TempDir(), "config"))
	service := newSettingsService(SettingsDeps{Store: settings.NewStore(settings.SettingsPath())})

	got, err := service.Appearance(t.Context())
	require.NoError(t, err)
	require.Empty(t, got.Theme)
	require.Equal(t, settings.DefaultTerminalFontSizePx, got.TerminalFontSizePx)
	require.Empty(t, got.CanvasFontSize)
	require.Empty(t, got.CanvasLineSpacing)
	require.Empty(t, got.CanvasPageWidth)
	require.True(t, got.TerminalShowWindows, "the terminal window listing ships on")
	require.True(t, got.TerminalShowStatusBar, "the session status bar ships on")
	require.Equal(t, 3, got.TerminalPoolSize, "the attach pool ships at three sessions")
}

func TestSettingsServiceSetAppearanceSettingsPreservesUnrelatedFields(t *testing.T) {
	t.Setenv(settings.EnvConfigDir, filepath.Join(t.TempDir(), "config"))
	cfg := settings.DefaultSettings()
	cfg.Updates.Enabled = false
	require.NoError(t, settings.SaveSettings(cfg))

	service := newSettingsService(SettingsDeps{Store: settings.NewStore(settings.SettingsPath())})
	require.NoError(t, service.SetTheme(t.Context(), "midnight"))
	require.NoError(t, service.SetTerminalFontSize(t.Context(), 14))
	require.NoError(t, service.SetCanvasFontSize(t.Context(), "xl"))
	require.NoError(t, service.SetCanvasLineSpacing(t.Context(), "relaxed"))
	require.NoError(t, service.SetCanvasPageWidth(t.Context(), "full"))
	require.NoError(t, service.SetTerminalShowWindows(t.Context(), false))
	require.NoError(t, service.SetTerminalPoolSize(t.Context(), 5))

	got, err := settings.LoadSettings()
	require.NoError(t, err)
	require.Equal(t, "midnight", got.Appearance.Theme)
	require.Equal(t, 14, got.Appearance.TerminalFontSize)
	require.Equal(t, "xl", got.Appearance.CanvasFontSize)
	require.Equal(t, "relaxed", got.Appearance.CanvasLineSpacing)
	require.Equal(t, "full", got.Appearance.CanvasPageWidth)
	require.False(t, got.Appearance.TerminalShowWindows)
	require.Equal(t, 5, got.Appearance.TerminalPoolSize)
	require.Equal(t, 5*time.Minute, got.Polling.Interval.Duration())
	require.False(t, got.Updates.Enabled)

	roundTripped, err := service.Appearance(t.Context())
	require.NoError(t, err)
	require.Equal(t, "midnight", roundTripped.Theme)
	require.Equal(t, 14, roundTripped.TerminalFontSizePx, "one appearance setter must not clobber the other field")
	require.Equal(t, "xl", roundTripped.CanvasFontSize)
	require.Equal(t, "relaxed", roundTripped.CanvasLineSpacing)
	require.Equal(t, "full", roundTripped.CanvasPageWidth)
	require.False(t, roundTripped.TerminalShowWindows)
	require.Equal(t, 5, roundTripped.TerminalPoolSize)
}

func TestSettingsServiceTerminalFontSizeReadsALegacyName(t *testing.T) {
	t.Setenv(settings.EnvConfigDir, filepath.Join(t.TempDir(), "config"))
	path := settings.SettingsPath()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("version: 5\nappearance:\n  terminal_font_size: large\n"), 0o600))
	service := newSettingsService(SettingsDeps{Store: settings.NewStore(path)})

	got, err := service.Appearance(t.Context())
	require.NoError(t, err)
	require.Equal(t, 14, got.TerminalFontSizePx)

	require.NoError(t, service.SetTerminalFontSize(t.Context(), 16))
	stored, err := settings.LoadSettings()
	require.NoError(t, err)
	require.Equal(t, 16, stored.Appearance.TerminalFontSize)
}

func TestSettingsServiceTerminalShowWindowsOffSurvivesUnrelatedSaves(t *testing.T) {
	t.Setenv(settings.EnvConfigDir, filepath.Join(t.TempDir(), "config"))
	service := newSettingsService(SettingsDeps{Store: settings.NewStore(settings.SettingsPath())})

	require.NoError(t, service.SetTerminalShowWindows(t.Context(), false))
	// False is the only non-default appearance value here, so the section is
	// all-zero — a save that omitted it would resurrect the default.
	require.NoError(t, service.SetKeybindings(t.Context(), map[string][]string{"feed.next": {"j"}}))

	got, err := service.Appearance(t.Context())
	require.NoError(t, err)
	require.False(t, got.TerminalShowWindows)
}
