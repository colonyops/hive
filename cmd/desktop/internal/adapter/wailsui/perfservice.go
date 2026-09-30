package wailsui

import (
	"context"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

type PerfService struct {
	perf *app.PerfService
}

func NewPerfService(perf *app.PerfService) *PerfService {
	return &PerfService{perf: perf}
}

// PerfSample is one completed span. AtUnixMs is the wall-clock start in
// milliseconds because that is what Date.now() gives the frontend; zero means
// "stamp it on arrival".
type PerfSample struct {
	AtUnixMs   int64          `json:"atUnixMs"`
	Scope      string         `json:"scope"`
	Name       string         `json:"name"`
	DurationMs float64        `json:"durationMs"`
	Attrs      map[string]any `json:"attrs,omitempty"`
	ID         string         `json:"id,omitempty"`
	Parent     string         `json:"parent,omitempty"`
}

type PerfInfo struct {
	Enabled  bool   `json:"enabled"`
	Path     string `json:"path"`
	MaxBytes int64  `json:"maxBytes"`
}

type PerfRecordResult struct {
	Written int `json:"written"`
}

func (s *PerfService) Info(ctx context.Context) PerfInfo {
	info := s.perf.Info(ctx)
	return PerfInfo{Enabled: info.Enabled, Path: info.Path, MaxBytes: info.MaxBytes}
}

func (s *PerfService) Record(ctx context.Context, samples []PerfSample) (PerfRecordResult, error) {
	converted := make([]app.PerfSample, 0, len(samples))
	for _, in := range samples {
		var at time.Time
		if in.AtUnixMs > 0 {
			at = time.UnixMilli(in.AtUnixMs).UTC()
		}
		converted = append(converted, app.PerfSample{
			At:         at,
			Scope:      in.Scope,
			Name:       in.Name,
			DurationMs: in.DurationMs,
			Attrs:      in.Attrs,
			ID:         in.ID,
			Parent:     in.Parent,
		})
	}

	written, err := s.perf.Record(ctx, converted)
	if err != nil {
		return PerfRecordResult{}, err
	}
	return PerfRecordResult{Written: written}, nil
}
