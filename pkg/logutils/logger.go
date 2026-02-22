package logutils

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
)

// New returns a new logger.
//
// If file is set, logs are written to that file in console format.
// If file is empty, logs are written to stdout in JSON format.
//
// The level parameter can be one of: debug, info, warn, error, fatal.
func New(level string, file string) (zerolog.Logger, func(), error) {
	closer := func() {}

	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		return zerolog.Logger{}, closer, err
	}

	var writer io.Writer = os.Stdout
	if file != "" {
		logsDir := filepath.Dir(file)
		if err := os.MkdirAll(logsDir, 0o755); err != nil {
			return zerolog.Logger{}, closer, fmt.Errorf("create logs dir: %w", err)
		}

		osFile, err := os.Create(file)
		if err != nil {
			return zerolog.Logger{}, closer, err
		}
		closer = func() { _ = osFile.Close() }
		writer = zerolog.ConsoleWriter{
			Out:        osFile,
			NoColor:    true,
			TimeFormat: time.RFC3339,
		}
	}

	l := zerolog.New(writer).
		With().
		Timestamp().
		Logger().
		Level(lvl)

	return l, closer, nil
}
