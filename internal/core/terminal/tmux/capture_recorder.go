package tmux

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/colonyops/hive/internal/core/terminal"
)

const (
	captureRecordSchemaVersion = 2
	weakLabelSource            = "hive_state_tracker_v1"
	identityKeyFilename        = ".identity.key"
)

// CaptureObservation is a fresh agent-pane capture and its inferred status.
type CaptureObservation struct {
	SessionName string
	PaneID      string
	Tool        string
	Content     string
	Status      terminal.Status
}

// CaptureRecorder records fresh pane captures for offline model training.
type CaptureRecorder interface {
	Record(observation CaptureObservation) error
}

// JSONCaptureRecorder writes content-addressed pane captures to local JSON files.
type JSONCaptureRecorder struct {
	dir         string
	runID       string
	identityKey []byte
}

type captureRecord struct {
	SchemaVersion   int             `json:"schema_version"`
	CapturedAt      time.Time       `json:"captured_at"`
	RunID           string          `json:"run_id"`
	SessionKey      string          `json:"session_key"`
	PaneKey         string          `json:"pane_key"`
	Tool            string          `json:"tool,omitempty"`
	Content         string          `json:"content"`
	ContentSHA256   string          `json:"content_sha256"`
	WeakLabel       terminal.Status `json:"weak_label"`
	WeakLabelSource string          `json:"weak_label_source"`
}

// NewJSONCaptureRecorder creates a content-addressed capture recorder.
func NewJSONCaptureRecorder(dir string) (*JSONCaptureRecorder, error) {
	if dir == "" {
		return nil, fmt.Errorf("capture recording directory is required")
	}
	if err := ensurePrivateRecordingDir(dir); err != nil {
		return nil, err
	}

	identityKey, err := loadOrCreateIdentityKey(dir)
	if err != nil {
		return nil, err
	}
	runID, err := randomHex(16)
	if err != nil {
		return nil, fmt.Errorf("generate capture recording run ID: %w", err)
	}
	return &JSONCaptureRecorder{dir: dir, runID: runID, identityKey: identityKey}, nil
}

// Dir returns the recorder's local capture directory.
func (r *JSONCaptureRecorder) Dir() string {
	if r == nil {
		return ""
	}
	return r.dir
}

// Record writes a capture unless its content hash already exists in the directory.
// Metadata belongs to the first observation of unique content; later observations
// with identical bytes are intentionally not sampled.
func (r *JSONCaptureRecorder) Record(observation CaptureObservation) error {
	if r == nil {
		return nil
	}

	fullHash := sha256.Sum256([]byte(observation.Content))
	hash := hex.EncodeToString(fullHash[:])
	path := filepath.Join(r.dir, hash+".json")
	exists, err := privateRegularFileExists(path)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	record := captureRecord{
		SchemaVersion:   captureRecordSchemaVersion,
		CapturedAt:      time.Now().UTC(),
		RunID:           r.runID,
		SessionKey:      r.opaqueKey("session", observation.SessionName),
		PaneKey:         r.opaqueKey("pane", observation.SessionName, observation.PaneID),
		Tool:            observation.Tool,
		Content:         observation.Content,
		ContentSHA256:   hash,
		WeakLabel:       observation.Status,
		WeakLabelSource: weakLabelSource,
	}
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode capture record: %w", err)
	}
	data = append(data, '\n')
	_, err = installPrivateFile(r.dir, path, data)
	return err
}

func ensurePrivateRecordingDir(dir string) error {
	info, err := os.Lstat(dir)
	switch {
	case err == nil:
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("capture recording directory must not be a symlink: %s", dir)
		}
		if !info.IsDir() {
			return fmt.Errorf("capture recording path is not a directory: %s", dir)
		}
	case errors.Is(err, os.ErrNotExist):
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create capture recording directory: %w", err)
		}
		info, err = os.Lstat(dir)
		if err != nil {
			return fmt.Errorf("inspect capture recording directory: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("capture recording path is not a private directory: %s", dir)
		}
	case err != nil:
		return fmt.Errorf("inspect capture recording directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("secure capture recording directory: %w", err)
	}
	return nil
}

func loadOrCreateIdentityKey(dir string) ([]byte, error) {
	path := filepath.Join(dir, identityKeyFilename)
	key, err := readIdentityKey(path)
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	key = make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate capture recording identity key: %w", err)
	}
	created, err := installPrivateFile(dir, path, key)
	if err != nil {
		return nil, fmt.Errorf("install capture recording identity key: %w", err)
	}
	if !created {
		return readIdentityKey(path)
	}
	return key, nil
}

func readIdentityKey(path string) ([]byte, error) {
	exists, err := privateRegularFileExists(path)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, os.ErrNotExist
	}
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read capture recording identity key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("capture recording identity key has invalid length: %d", len(key))
	}
	return key, nil
}

func privateRegularFileExists(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect private recording file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false, fmt.Errorf("private recording path is not a regular file: %s", path)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return false, fmt.Errorf("secure private recording file: %w", err)
	}
	return true, nil
}

// installPrivateFile atomically creates finalPath without replacing existing data.
func installPrivateFile(dir, finalPath string, data []byte) (bool, error) {
	temp, err := os.CreateTemp(dir, ".capture-*.tmp")
	if err != nil {
		return false, fmt.Errorf("create temporary recording file: %w", err)
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return false, fmt.Errorf("secure temporary recording file: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return false, fmt.Errorf("write temporary recording file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return false, fmt.Errorf("close temporary recording file: %w", err)
	}
	if err := os.Link(tempPath, finalPath); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return false, fmt.Errorf("install recording file: %w", err)
		}
		if _, err := privateRegularFileExists(finalPath); err != nil {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

func (r *JSONCaptureRecorder) opaqueKey(namespace string, values ...string) string {
	mac := hmac.New(sha256.New, r.identityKey)
	_, _ = io.WriteString(mac, namespace)
	for _, value := range values {
		_, _ = io.WriteString(mac, "\x00")
		_, _ = io.WriteString(mac, value)
	}
	return hex.EncodeToString(mac.Sum(nil)[:16])
}

func randomHex(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
