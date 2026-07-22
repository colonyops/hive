package main

import (
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/desktop"
	"github.com/colonyops/hive/internal/desktop/feed"
	"github.com/colonyops/hive/internal/desktop/pipeline"
)

// SettingsService exposes user-tunable desktop settings to the frontend.
// Saving persists settings.yaml and applies supported values to the running
// services immediately.
type SettingsService struct {
	producer *pipeline.Producer
	fetcher  *feed.LiveProvider
	logger   zerolog.Logger
}

// NewSettingsService constructs the Wails settings binding. producer and
// fetcher are nil in mock mode, while settings persistence remains available.
func NewSettingsService(producer *pipeline.Producer, fetcher *feed.LiveProvider, logger zerolog.Logger) *SettingsService {
	return &SettingsService{producer: producer, fetcher: fetcher, logger: logger}
}

// GithubSettings is the GitHub integration's editable configuration.
type GithubSettings struct {
	PollIntervalSeconds    int `json:"pollIntervalSeconds"`
	MinPollIntervalSeconds int `json:"minPollIntervalSeconds"`
}

// GithubSettings returns the current resolved GitHub polling settings.
func (s *SettingsService) GithubSettings() (GithubSettings, error) {
	settings, err := desktop.LoadSettings()
	if err != nil {
		return GithubSettings{}, err
	}
	interval, err := settings.PollIntervalOrDefault(feed.DefaultPollInterval)
	if err != nil {
		return GithubSettings{}, err
	}
	return GithubSettings{
		PollIntervalSeconds:    int(interval / time.Second),
		MinPollIntervalSeconds: int(desktop.MinPollInterval / time.Second),
	}, nil
}

// SetGithubSettings validates, persists, and immediately applies the GitHub
// poll interval. API callers below the floor are rejected rather than clamped.
func (s *SettingsService) SetGithubSettings(settings GithubSettings) error {
	minimum := int(desktop.MinPollInterval / time.Second)
	if settings.PollIntervalSeconds < minimum {
		return fmt.Errorf("poll interval must be at least %d seconds", minimum)
	}
	if uint64(settings.PollIntervalSeconds) > uint64((time.Duration(1<<63-1))/time.Second) {
		return fmt.Errorf("poll interval is too large")
	}

	interval := time.Duration(settings.PollIntervalSeconds) * time.Second
	// Load-modify-save so unrelated fields (e.g. AutoUpdate) are preserved
	// rather than clobbered by writing a fresh, single-field Settings value.
	current, err := desktop.LoadSettings()
	if err != nil {
		return err
	}
	current.PollInterval = interval.String()
	if err := desktop.SaveSettings(current); err != nil {
		return err
	}
	if s.producer != nil {
		s.producer.SetInterval(interval)
	}
	if s.fetcher != nil {
		s.fetcher.SetSearchTTL(interval)
	}
	return nil
}
