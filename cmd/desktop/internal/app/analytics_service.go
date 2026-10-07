package app

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/colonyops/hive/internal/config"
	usage "github.com/colonyops/hive/internal/hive/usageanalytics"
	store "github.com/colonyops/hive/internal/store/usageanalytics"
)

type AnalyticsSummary struct {
	InstallationID      string        `json:"installationId"`
	Counts              store.Summary `json:"counts"`
	Enabled             bool          `json:"enabled"`
	EffectiveEnabled    bool          `json:"effectiveEnabled"`
	Active              bool          `json:"active"`
	RestartNeeded       bool          `json:"restartNeeded"`
	EnvironmentOverride bool          `json:"environmentOverride"`
	RetentionDays       int           `json:"retentionDays"`
}

type AnalyticsService struct {
	mu                  sync.Mutex
	dataDir             string
	location            func() HiveConfigLocation
	environmentDisabled bool
	startupEnabled      bool
	recorder            *usage.Service
}

func (s *AnalyticsService) Summary(ctx context.Context) (AnalyticsSummary, error) {
	cfg, err := config.Load(s.location().Path, s.dataDir)
	if err != nil {
		return AnalyticsSummary{}, Wrap(err, KindInternal, "read analytics configuration")
	}
	counts, installationID, err := store.ReadSummary(ctx, s.dataDir, time.Now().UTC())
	if err != nil {
		return AnalyticsSummary{}, Wrap(err, KindInternal, "read usage history")
	}
	enabled := cfg.Analytics.CollectionEnabled()
	effective := enabled && !s.environmentDisabled
	return AnalyticsSummary{InstallationID: installationID, Counts: counts, Enabled: enabled, EffectiveEnabled: s.startupEnabled, Active: s.recorder.Active(), RestartNeeded: effective != s.startupEnabled, EnvironmentOverride: s.environmentDisabled, RetentionDays: store.RetentionDays}, nil
}

func (s *AnalyticsService) SetEnabled(ctx context.Context, enabled bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	check := func(path string) error { _, err := config.Load(path, s.dataDir); return err }
	if err := config.SetAnalyticsEnabled(s.location().Path, enabled, check); err != nil {
		return Wrap(err, hiveWriteKind(err), "save analytics configuration")
	}
	return nil
}

func (s *AnalyticsService) Clear(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// No history means nothing to clear, including no installation identifier to rotate.
	if _, err := os.Stat(filepath.Join(s.dataDir, store.Filename)); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return Wrap(err, KindInternal, "inspect usage history")
	}
	history, err := store.Open(ctx, s.dataDir)
	if err != nil {
		return Wrap(err, KindInternal, "open usage history")
	}
	defer func() { _ = history.Close() }()
	cutoff, err := history.Clear(ctx)
	if err != nil {
		return Wrap(err, KindInternal, "clear usage history")
	}
	s.recorder.DiscardBefore(cutoff)
	return nil
}
