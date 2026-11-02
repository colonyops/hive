// Package desktopendpoint records where a running Hive Desktop serves its
// loopback HTTP server, so the hive CLI can find it without configuration.
package desktopendpoint

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/colonyops/hive/pkg/atomicfile"
)

// ErrNotRunning means no endpoint file exists: Hive Desktop is not running, or
// it has not started its HTTP server yet.
var ErrNotRunning = errors.New("hive desktop is not running")

// Endpoint is the content of the endpoint file.
type Endpoint struct {
	// URL is the base URL of the loopback server, without a trailing path.
	URL       string    `json:"url"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"startedAt"`
}

// Path returns the endpoint file path under the hive data directory.
func Path(dataDir string) string {
	return filepath.Join(dataDir, "desktop", "endpoint.json")
}

// Write stores e atomically, so a concurrent Read sees the old file or the new
// one and never a partial write.
func Write(dataDir string, e Endpoint) error {
	path := Path(dataDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create endpoint directory: %w", err)
	}
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encode endpoint: %w", err)
	}
	if err := atomicfile.Write(path, data, 0o600); err != nil {
		return fmt.Errorf("write endpoint: %w", err)
	}
	return nil
}

// Remove deletes the endpoint file. A missing file is not an error.
func Remove(dataDir string) error {
	if err := os.Remove(Path(dataDir)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove endpoint: %w", err)
	}
	return nil
}

// Read loads the endpoint file. It returns an error wrapping ErrNotRunning
// when the file is missing. It does not check that the PID is alive: a file
// left by a crash surfaces as a connection error to the caller.
func Read(dataDir string) (Endpoint, error) {
	data, err := os.ReadFile(Path(dataDir))
	if errors.Is(err, fs.ErrNotExist) {
		return Endpoint{}, fmt.Errorf("%w: no %s", ErrNotRunning, Path(dataDir))
	}
	if err != nil {
		return Endpoint{}, fmt.Errorf("read endpoint: %w", err)
	}
	var e Endpoint
	if err := json.Unmarshal(data, &e); err != nil {
		return Endpoint{}, fmt.Errorf("parse %s: %w", Path(dataDir), err)
	}
	if e.URL == "" {
		return Endpoint{}, fmt.Errorf("parse %s: url is empty", Path(dataDir))
	}
	return e, nil
}
