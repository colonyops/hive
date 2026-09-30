package app

import (
	"context"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/perf"
)

// PerfService takes performance spans from the UI and appends them to the
// recorder's file. It is a development facility: when development.perf.enabled
// is off the recorder is disabled, Info reports that, and the frontend stops
// buffering rather than sending batches nothing will write.
type PerfService struct {
	recorder *perf.Recorder
	logger   zerolog.Logger
}

func newPerfService(recorder *perf.Recorder, logger zerolog.Logger) *PerfService {
	return &PerfService{recorder: recorder, logger: logger}
}

// openPerfRecorder degrades to a disabled recorder when the file cannot be
// opened. Losing UI timings is never a reason to fail startup, and the failure
// still reaches the log and Info().Enabled.
func openPerfRecorder(enabled bool, stateDir string, logger zerolog.Logger) *perf.Recorder {
	if !enabled {
		return perf.Off()
	}
	path := filepath.Join(stateDir, perf.FileName)
	recorder, err := perf.New(perf.Options{Path: path})
	if err != nil {
		logger.Warn().Err(err).Str("path", path).Msg("perf recording disabled: could not open the file")
		return perf.Off()
	}
	logger.Info().Str("path", path).Msg("UI performance recording enabled")
	return recorder
}

// PerfSample is one completed span. DurationMs comes from the caller's own
// clock — the browser's performance.now() — because a timestamp round-tripped
// through the RPC boundary would measure the boundary as much as the work.
type PerfSample struct {
	At         time.Time
	Scope      string
	Name       string
	DurationMs float64
	Attrs      map[string]any
	ID         string
	Parent     string
}

// PerfInfo tells the caller whether to instrument at all, and where the file
// is so an agent can read it without knowing the state-directory layout.
type PerfInfo struct {
	Enabled  bool
	Path     string
	MaxBytes int64
}

func (s *PerfService) Info(_ context.Context) PerfInfo {
	return PerfInfo{
		Enabled:  s.recorder.Enabled(),
		Path:     s.recorder.Path(),
		MaxBytes: s.recorder.MaxBytes(),
	}
}

// Record appends a batch and reports how many samples landed. A batch sent to
// a disabled recorder is not an error the UI should surface — instrumentation
// left in the code is expected to run against a build with recording off.
func (s *PerfService) Record(_ context.Context, samples []PerfSample) (int, error) {
	if !s.recorder.Enabled() {
		return 0, nil
	}

	converted := make([]perf.Sample, 0, len(samples))
	for _, in := range samples {
		converted = append(converted, perf.Sample{
			At:         in.At,
			Scope:      in.Scope,
			Name:       in.Name,
			DurationMs: in.DurationMs,
			Attrs:      in.Attrs,
			ID:         in.ID,
			Parent:     in.Parent,
		})
	}

	written, err := s.recorder.Record(converted...)
	if err != nil {
		return written, Wrap(err, KindInternal, "recording performance samples")
	}
	if dropped := len(samples) - written; dropped > 0 {
		s.logger.Debug().Int("dropped", dropped).Msg("perf: samples failed validation")
	}
	return written, nil
}

func (s *PerfService) Close() error { return s.recorder.Close() }
