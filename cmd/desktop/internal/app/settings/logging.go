package settings

import (
	"fmt"
	"io"
	"os"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/platform/observe"
	"github.com/colonyops/hive/pkg/logutils"
)

const EnvLogLevel = "HIVE_DESKTOP_LOG_LEVEL"

// ResolveLogLevel validates the process log-level override once at startup.
func ResolveLogLevel() (zerolog.Level, error) {
	value := os.Getenv(EnvLogLevel)
	if value == "" {
		return zerolog.InfoLevel, nil
	}
	level, err := zerolog.ParseLevel(value)
	if err != nil {
		return zerolog.InfoLevel, fmt.Errorf("parse %s: %w", EnvLogLevel, err)
	}
	return level, nil
}

// NewLogger builds the root logger at the resolved immutable path and level.
// It logs to stderr as well as the file, and when the file is unavailable it
// returns the error with a logger that still writes to stderr and extra.
//
// An extra writer receives the encoded JSON event, not the console rendering,
// which is the seam a log bridge attaches to: a zerolog.Hook sees only level
// and message.
func NewLogger(serviceName, path string, level zerolog.Level, extra ...io.Writer) (zerolog.Logger, func(), error) {
	return logutils.NewRoot(logutils.Options{
		Service: serviceName,
		Level:   level,
		File:    path,
		Console: os.Stderr,
		JSON:    extra,
		// The hook adds nothing to an event with no span, and whether the ids
		// mean anything is telemetry's business.
		Hooks: []zerolog.Hook{observe.TraceHook},
	})
}
