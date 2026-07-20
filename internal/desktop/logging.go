package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
)

// logFileName is the desktop app's log file, kept alongside the desktop state
// (read-state, pipeline db) rather than the CLI's <data-dir>/hive.log so the
// two apps' logs stay separate.
const logFileName = "desktop.log"

// LogFile is the desktop app's log file path: <state-dir>/desktop.log.
func LogFile() string {
	return filepath.Join(StateDir(), logFileName)
}

// NewLogger builds the desktop app's root logger. It writes to LogFile() in
// console format AND tees to stderr, so `wails3 dev` keeps showing logs in the
// terminal while a running app still has a file the System settings screen can
// open. The level comes from HIVE_LOG_LEVEL (default info).
//
// If the log file cannot be opened, it degrades to stderr-only rather than
// failing app startup; the returned error is informational.
func NewLogger() (zerolog.Logger, func(), error) {
	level := zerolog.InfoLevel
	if lvl, err := zerolog.ParseLevel(os.Getenv("HIVE_LOG_LEVEL")); err == nil && os.Getenv("HIVE_LOG_LEVEL") != "" {
		level = lvl
	}

	stderr := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}

	path := LogFile()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		l := zerolog.New(stderr).With().Timestamp().Logger().Level(level)
		return l, func() {}, fmt.Errorf("create desktop log dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		l := zerolog.New(stderr).With().Timestamp().Logger().Level(level)
		return l, func() {}, fmt.Errorf("open desktop log file: %w", err)
	}

	fileW := zerolog.ConsoleWriter{Out: f, NoColor: true, TimeFormat: time.RFC3339}
	l := zerolog.New(zerolog.MultiLevelWriter(fileW, stderr)).With().Timestamp().Logger().Level(level)
	return l, func() { _ = f.Close() }, nil
}
