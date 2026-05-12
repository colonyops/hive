package logutils

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
)

// New returns a new logger.
//
// Logs are emitted as one structured JSON object per line. When file is set
// the JSON is appended to that file; otherwise it goes to stdout. The JSON
// format keeps log assertions (and external tools like jq) trivial in tests
// and downstream tooling — see hive timer's cap-bypass / fire-failure
// assertions.
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

		osFile, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
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
