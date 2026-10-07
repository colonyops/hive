package settings

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

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
//
// An extra writer is another arm of the MultiLevelWriter and receives the
// encoded JSON event, not the console rendering — which is the seam a log
// bridge attaches to, since a zerolog.Hook sees only level and message. An
// extra arm must not fail the write or block.
func NewLogger(serviceName, path string, level zerolog.Level, extra ...io.Writer) (zerolog.Logger, func(), error) {
	stderr := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
	build := func(writers ...io.Writer) zerolog.Logger {
		// Installed unconditionally: the hook adds nothing to an event with no
		// span, and whether the ids mean anything is telemetry's business.
		logger := zerolog.New(zerolog.MultiLevelWriter(writers...)).
			With().Timestamp().Logger().
			Level(level).
			Hook(observe.TraceHook)
		return logutils.Service(logger, serviceName)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return build(append([]io.Writer{stderr}, extra...)...), func() {}, fmt.Errorf("create log dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return build(append([]io.Writer{stderr}, extra...)...), func() {}, fmt.Errorf("open log file: %w", err)
	}
	fileW := zerolog.ConsoleWriter{Out: f, NoColor: true, TimeFormat: time.RFC3339}
	l := build(append([]io.Writer{fileW, stderr}, extra...)...)
	return l, func() { _ = f.Close() }, nil
}
