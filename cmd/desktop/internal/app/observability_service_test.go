package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

func telemetrySettingsFixture() settings.TelemetrySettings {
	return settings.TelemetrySettings{
		Enabled:    true,
		Endpoint:   "https://otlp-gateway.example.com/otlp",
		InstanceID: "1234",
		HostID:     "machine-a",
		Token:      "env:OTLP_TOKEN",
		Profiles: settings.ProfileTelemetrySettings{
			Enabled:  true,
			Endpoint: "https://profiles.example.com",
			User:     "5678",
			Token:    "env:PROFILES_TOKEN",
		},
	}
}

func TestObservabilitySettingsReportConfigurationAndStartupState(t *testing.T) {
	store := settings.NewStore(t.TempDir() + "/settings.yaml")
	configured := telemetrySettingsFixture()
	_, err := store.Update(func(current *settings.Settings) error {
		current.Telemetry = configured
		return nil
	})
	require.NoError(t, err)

	service := newObservabilityService(store, configured, TelemetryRuntime{OTLPRunning: true, ProfilesRunning: true})
	t.Cleanup(func() { require.NoError(t, service.close()) })

	view, err := service.Settings(t.Context())
	require.NoError(t, err)
	assert.Equal(t, ExportStatus{Enabled: true, Configured: true, Running: true}, view.OTLP)
	assert.Equal(t, ExportStatus{Enabled: true, Configured: true, Running: true}, view.Profiles)
	assert.Empty(t, view.StartError)
}

func TestObservabilitySettingsReportRestartRequiredPerDestination(t *testing.T) {
	store := settings.NewStore(t.TempDir() + "/settings.yaml")
	startup := telemetrySettingsFixture()
	current := startup
	current.Profiles.Endpoint = "https://profiles-2.example.com"
	_, err := store.Update(func(cfg *settings.Settings) error {
		cfg.Telemetry = current
		return nil
	})
	require.NoError(t, err)

	service := newObservabilityService(store, startup, TelemetryRuntime{OTLPRunning: true, ProfilesRunning: true})
	t.Cleanup(func() { require.NoError(t, service.close()) })

	view, err := service.Settings(t.Context())
	require.NoError(t, err)
	assert.False(t, view.OTLP.RestartRequired)
	assert.True(t, view.Profiles.RestartRequired)
}

func TestObservabilitySettingsReportDisabledPreparedDestinations(t *testing.T) {
	store := settings.NewStore(t.TempDir() + "/settings.yaml")
	prepared := telemetrySettingsFixture()
	prepared.Enabled = false
	prepared.Profiles.Enabled = false
	_, err := store.Update(func(cfg *settings.Settings) error {
		cfg.Telemetry = prepared
		return nil
	})
	require.NoError(t, err)

	service := newObservabilityService(store, prepared, TelemetryRuntime{StartError: "profiles unavailable"})
	t.Cleanup(func() { require.NoError(t, service.close()) })

	view, err := service.Settings(t.Context())
	require.NoError(t, err)
	assert.Equal(t, ExportStatus{Configured: true}, view.OTLP)
	assert.Equal(t, ExportStatus{Configured: true}, view.Profiles)
	assert.Equal(t, "profiles unavailable", view.StartError)
}
