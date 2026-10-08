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

// NewLogger builds the desktop root logger. An extra writer receives the
// encoded JSON event, the seam the telemetry log bridge attaches to.
func NewLogger(serviceName, path string, level zerolog.Level, extra ...io.Writer) (zerolog.Logger, func(), error) {
	return logutils.NewRoot(logutils.Options{
		Service: serviceName,
		Level:   level,
		File:    path,
		Console: os.Stderr,
		JSON:    extra,
		Hooks:   []zerolog.Hook{observe.TraceHook},
	})
}
