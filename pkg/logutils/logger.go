package logutils

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
)

// New returns a new logger that writes JSON to the specified file.
// If file is empty, logs are written to stdout.
//
// The level parameter can be one of: debug, info, warn, error, fatal.
func New(level string, file string) (zerolog.Logger, func(), error) {
	closer := func() {}

	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		return zerolog.Logger{}, closer, err
	}

	// File Setup
	writer := os.Stdout
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
		writer = osFile
	}

	l := zerolog.New(writer).
		With().
		Timestamp().
		Logger().
		Level(lvl)

	return l, closer, nil
}
