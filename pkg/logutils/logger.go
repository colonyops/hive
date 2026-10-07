package logutils

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// Options selects the sinks and identity of a root logger.
type Options struct {
	// Service is written as service_name on every event.
	Service string
	Level   zerolog.Level
	// File, when set, appends console-format lines without color to that path,
	// creating it and its parent directory as needed.
	File string
	// Console, when set, receives colored console-format lines.
	Console io.Writer
	// JSON writers receive each encoded JSON event as is. A writer must not
	// fail the write or block.
	JSON  []io.Writer
	Hooks []zerolog.Hook
}

// NewRoot builds a program's root logger and returns a cleanup that closes the
// file it opened. The cleanup is safe to call more than once.
//
// When File cannot be opened, NewRoot returns the error together with a usable
// logger over the remaining sinks, so the caller decides whether to continue.
// A logger with no sinks discards every event.
func NewRoot(opts Options) (zerolog.Logger, func(), error) {
	var (
		writers []io.Writer
		cleanup = func() {}
		fileErr error
	)

	if opts.File != "" {
		f, err := openAppend(opts.File)
		if err != nil {
			fileErr = err
		} else {
			cleanup = sync.OnceFunc(func() { _ = f.Close() })
			writers = append(writers, zerolog.ConsoleWriter{Out: f, NoColor: true, TimeFormat: time.RFC3339})
		}
	}
	if opts.Console != nil {
		writers = append(writers, zerolog.ConsoleWriter{Out: opts.Console, TimeFormat: time.RFC3339})
	}
	writers = append(writers, opts.JSON...)

	out := io.Discard
	if len(writers) > 0 {
		out = zerolog.MultiLevelWriter(writers...)
	}

	logger := zerolog.New(out).With().Timestamp().Logger().Level(opts.Level)
	for _, h := range opts.Hooks {
		logger = logger.Hook(h)
	}
	return Service(logger, opts.Service), cleanup, fileErr
}

func openAppend(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	return f, nil
}
