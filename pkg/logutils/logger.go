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

type Options struct {
	Service string
	Level   zerolog.Level
	File    string
	Console io.Writer
	// JSON writers must not block.
	JSON  []io.Writer
	Hooks []zerolog.Hook
}

// NewRoot builds a program's root logger. When File cannot be opened it
// returns the error and a logger over the remaining sinks. The cleanup is safe
// to call more than once.
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
