package app

import (
	"context"
	"os"

	"go.opentelemetry.io/otel/metric"

	"github.com/colonyops/hive/cmd/desktop/internal/app/procstats"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

// TelemetryRuntime reports which exporters this process started. Running means
// the SDK pipeline started, not that the remote backend has accepted a signal.
type TelemetryRuntime struct {
	OTLPRunning     bool
	ProfilesRunning bool
	StartError      string
}

// ExportStatus distinguishes valid local configuration from exporter startup.
// Running does not confirm that the backend accepted data.
type ExportStatus struct {
	Enabled         bool
	Configured      bool
	Running         bool
	RestartRequired bool
}

type ObservabilitySettings struct {
	OTLP       ExportStatus
	Profiles   ExportStatus
	StartError string
}

type ObservabilityService struct {
	store   *settings.Store
	sampler *procstats.Sampler
	metrics metric.Registration
	startup settings.TelemetrySettings
	runtime TelemetryRuntime
}

func newObservabilityService(store *settings.Store, startup settings.TelemetrySettings, runtime TelemetryRuntime) *ObservabilityService {
	sampler := procstats.New(int32(os.Getpid()))
	return &ObservabilityService{
		store:   store,
		sampler: sampler,
		metrics: sampler.RegisterMetrics(),
		startup: startup,
		runtime: runtime,
	}
}

func (s *ObservabilityService) Stats(ctx context.Context) (procstats.Stats, error) {
	return s.sampler.Sample(ctx)
}

func (s *ObservabilityService) Settings(context.Context) (ObservabilitySettings, error) {
	cfg, err := s.store.Effective()
	if err != nil {
		return ObservabilitySettings{}, Wrap(err, KindInternal, "reading observability settings")
	}
	return ObservabilitySettings{
		OTLP: ExportStatus{
			Enabled:         cfg.Telemetry.Enabled,
			Configured:      otlpConfigured(cfg.Telemetry),
			Running:         s.runtime.OTLPRunning,
			RestartRequired: otlpNeedsRestart(s.startup, cfg.Telemetry),
		},
		Profiles: ExportStatus{
			Enabled:         cfg.Telemetry.Profiles.Enabled,
			Configured:      profilesConfigured(cfg.Telemetry),
			Running:         s.runtime.ProfilesRunning,
			RestartRequired: profilesNeedRestart(s.startup, cfg.Telemetry),
		},
		StartError: s.runtime.StartError,
	}, nil
}

func otlpConfigured(telemetry settings.TelemetrySettings) bool {
	probe := settings.DefaultSettings()
	probe.Telemetry = telemetry
	probe.Telemetry.Enabled = true
	probe.Telemetry.Profiles.Enabled = false
	return probe.Validate() == nil
}

func profilesConfigured(telemetry settings.TelemetrySettings) bool {
	probe := settings.DefaultSettings()
	probe.Telemetry = telemetry
	probe.Telemetry.Enabled = false
	probe.Telemetry.Profiles.Enabled = true
	return probe.Validate() == nil
}

func otlpNeedsRestart(startup, current settings.TelemetrySettings) bool {
	if startup.Enabled != current.Enabled {
		return true
	}
	return current.Enabled && (startup.Endpoint != current.Endpoint || startup.InstanceID != current.InstanceID || startup.Token != current.Token || startup.HostID != current.HostID)
}

func profilesNeedRestart(startup, current settings.TelemetrySettings) bool {
	if startup.Profiles.Enabled != current.Profiles.Enabled {
		return true
	}
	return current.Profiles.Enabled && (startup.Profiles.Endpoint != current.Profiles.Endpoint || startup.Profiles.User != current.Profiles.User || startup.Profiles.Token != current.Profiles.Token || startup.HostID != current.HostID)
}

func (s *ObservabilityService) close() error {
	return s.metrics.Unregister()
}
