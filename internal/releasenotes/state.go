package releasenotes

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// StateFileName is the marker file's name under the state directory.
const StateFileName = "releasenotes.json"

// State records the newest version whose notes the user has already seen.
//
// It lives under the state directory rather than in settings.yaml because
// settings.yaml is dotfiles-managed and shared between machines: "I have read
// these notes" is a fact about one installation, not a preference to carry
// across them. Acknowledging on a laptop must not suppress the notes on a
// desktop that has yet to be updated.
type State struct {
	path string
	mu   sync.Mutex
}

// NewState builds the marker over stateDir. The file is created on first
// write; a missing file reads as "nothing acknowledged yet", which is how a
// fresh install is told apart from an upgrade.
func NewState(stateDir string) *State {
	return &State{path: filepath.Join(stateDir, StateFileName)}
}

type stateFile struct {
	AcknowledgedVersion string `json:"acknowledged_version"`
}

// Acknowledged returns the recorded version, or "" when none has been
// recorded. A corrupt file reads as "" rather than failing the launch, and ""
// is indistinguishable from a fresh install — so the cost is that the running
// version is silently adopted and its notes are never shown. Losing one
// release's notes is a far smaller price than refusing to start.
func (s *State) Acknowledged() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.path)
	if err != nil {
		return ""
	}
	var file stateFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return ""
	}
	return file.AcknowledgedVersion
}

// Acknowledge records version as seen, replacing the file atomically so a
// crash mid-write cannot leave a truncated marker behind.
func (s *State) Acknowledge(version string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	contents, err := json.Marshal(stateFile{AcknowledgedVersion: version})
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(s.path), StateFileName+".*")
	if err != nil {
		return fmt.Errorf("stage release notes marker: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(append(contents, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write release notes marker: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write release notes marker: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("replace release notes marker: %w", err)
	}
	return nil
}
