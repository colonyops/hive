package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/procstats"
)

type ObservabilityService struct {
	observability *app.ObservabilityService
}

func NewObservabilityService(observability *app.ObservabilityService) *ObservabilityService {
	return &ObservabilityService{observability: observability}
}

type RuntimeStats struct {
	SampledAtUnixMs   int64               `json:"sampledAtUnixMs"`
	UptimeMs          int64               `json:"uptimeMs"`
	Process           procstats.Process   `json:"process"`
	Children          []procstats.Process `json:"children"`
	ChildrenTruncated bool                `json:"childrenTruncated"`
	TotalRSSBytes     uint64              `json:"totalRssBytes"`
	TotalCPUPercent   float64             `json:"totalCpuPercent"`
	Go                procstats.GoRuntime `json:"go"`
}

func (s *ObservabilityService) Stats(ctx context.Context) (RuntimeStats, error) {
	sample, err := s.observability.Stats(ctx)
	if err != nil {
		return RuntimeStats{}, err
	}
	return RuntimeStats{
		SampledAtUnixMs:   sample.SampledAt.UnixMilli(),
		UptimeMs:          sample.UptimeMs,
		Process:           sample.Process,
		Children:          sample.Children,
		ChildrenTruncated: sample.ChildrenTruncated,
		TotalRSSBytes:     sample.TotalRSSBytes(),
		TotalCPUPercent:   sample.TotalCPUPercent(),
		Go:                sample.Go,
	}, nil
}

// ExportStatus distinguishes valid local configuration from exporter startup.
// Running does not confirm that the backend accepted data.
type ExportStatus struct {
	Enabled         bool `json:"enabled"`
	Configured      bool `json:"configured"`
	Running         bool `json:"running"`
	RestartRequired bool `json:"restartRequired"`
}

type ObservabilitySettings struct {
	OTLP       ExportStatus `json:"otlp"`
	Profiles   ExportStatus `json:"profiles"`
	StartError string       `json:"startError"`
}

func (s *ObservabilityService) Settings(ctx context.Context) (ObservabilitySettings, error) {
	current, err := s.observability.Settings(ctx)
	if err != nil {
		return ObservabilitySettings{}, err
	}
	return ObservabilitySettings{
		OTLP: ExportStatus{
			Enabled:         current.OTLP.Enabled,
			Configured:      current.OTLP.Configured,
			Running:         current.OTLP.Running,
			RestartRequired: current.OTLP.RestartRequired,
		},
		Profiles: ExportStatus{
			Enabled:         current.Profiles.Enabled,
			Configured:      current.Profiles.Configured,
			Running:         current.Profiles.Running,
			RestartRequired: current.Profiles.RestartRequired,
		},
		StartError: current.StartError,
	}, nil
}
