package main

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/desktop"
	"github.com/colonyops/hive/internal/desktop/feed"
	"github.com/colonyops/hive/internal/desktop/pipeline"
	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
	"github.com/colonyops/hive/internal/github"
)

func TestSettingsServiceSetGithubSettingsRejectsBelowFloor(t *testing.T) {
	t.Setenv(desktop.EnvConfigPath, filepath.Join(t.TempDir(), "config", "profiles.yaml"))
	service := NewSettingsService(nil, nil, zerolog.Nop())

	err := service.SetGithubSettings(GithubSettings{PollIntervalSeconds: int(desktop.MinPollInterval/time.Second) - 1})
	require.Error(t, err)
}

func TestSettingsServiceSetGithubSettingsPreservesAutoUpdate(t *testing.T) {
	t.Setenv(desktop.EnvConfigPath, filepath.Join(t.TempDir(), "config", "profiles.yaml"))
	// Seed an explicit auto_update:false alongside a poll interval.
	disabled := false
	require.NoError(t, desktop.SaveSettings(desktop.Settings{PollInterval: "5m", AutoUpdate: &disabled}))

	service := NewSettingsService(nil, nil, zerolog.Nop())
	require.NoError(t, service.SetGithubSettings(GithubSettings{PollIntervalSeconds: 120}))

	got, err := desktop.LoadSettings()
	require.NoError(t, err)
	require.Equal(t, "2m0s", got.PollInterval)
	require.NotNil(t, got.AutoUpdate, "auto_update must survive a poll-interval save")
	require.False(t, *got.AutoUpdate)
}

type settingsServiceSource struct {
	mu    sync.Mutex
	calls int
}

func (s *settingsServiceSource) Produce(_ context.Context, _ func(pipeline.Msg) error) error {
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
		t.Setenv(desktop.EnvConfigPath, filepath.Join(t.TempDir(), "config", "profiles.yaml"))
		provider := feed.NewLiveProvider(github.NewClient(), nil, zerolog.Nop())
		db, err := pipelinedb.Open(t.TempDir(), pipelinedb.DefaultOpenOptions())
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })
		source := &settingsServiceSource{}
		producer := pipeline.NewProducer(db, func(context.Context) (map[string]pipeline.Source, error) {
			return map[string]pipeline.Source{"github": source}, nil
		}, time.Hour, nil, zerolog.Nop())
		service := NewSettingsService(producer, provider, zerolog.Nop())

		require.NoError(t, service.SetGithubSettings(GithubSettings{PollIntervalSeconds: 120}))
		settings, err := desktop.LoadSettings()
		require.NoError(t, err)
		require.Equal(t, "2m0s", settings.PollInterval)

		got, err := service.GithubSettings()
		require.NoError(t, err)
		require.Equal(t, 120, got.PollIntervalSeconds)
		require.Equal(t, 60, got.MinPollIntervalSeconds)

		producer.Start()
		time.Sleep(2 * time.Minute)
		synctest.Wait()
		producer.Stop()
		require.Equal(t, 1, source.callCount(), "saved settings reset the live producer cadence")
	})
}
