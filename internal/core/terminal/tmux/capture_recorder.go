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
	"sync"
	"time"
	"unicode/utf8"

	"github.com/colonyops/hive/internal/core/terminal"
)

const (
	captureRecordSchemaVersion = 1
	maxRecordedContentBytes    = 8 * 1024
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

// JSONLCaptureRecorder writes privacy-conscious pane captures to a local JSONL file.
type JSONLCaptureRecorder struct {
	mu          sync.Mutex
	path        string
	runID       string
	identityKey []byte
	lastHash    map[string][sha256.Size]byte
}

type captureRecord struct {
	SchemaVersion    int             `json:"schema_version"`
	CapturedAt       time.Time       `json:"captured_at"`
	RunID            string          `json:"run_id"`
	SessionKey       string          `json:"session_key"`
	PaneKey          string          `json:"pane_key"`
	Tool             string          `json:"tool,omitempty"`
	Content          string          `json:"content"`
	ContentSHA256    string          `json:"content_sha256"`
	ContentTruncated bool            `json:"content_truncated"`
	WeakLabel        terminal.Status `json:"weak_label"`
	WeakLabelSource  string          `json:"weak_label_source"`
}

// NewJSONLCaptureRecorder creates a recorder with a unique output file.
func NewJSONLCaptureRecorder(dir string) (*JSONLCaptureRecorder, error) {
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
	filename := fmt.Sprintf("captures-%s-%d-%s.jsonl", time.Now().UTC().Format("20060102T150405Z"), os.Getpid(), runID[:8])
	path := filepath.Join(dir, filename)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create capture recording: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close new capture recording: %w", err)
	}

	return &JSONLCaptureRecorder{
		path:        path,
		runID:       runID,
		identityKey: identityKey,
		lastHash:    make(map[string][sha256.Size]byte),
	}, nil
}

// Path returns the recorder's local JSONL output path.
func (r *JSONLCaptureRecorder) Path() string {
	if r == nil {
		return ""
	}
	return r.path
}

// Record appends a changed capture to the recorder's JSONL file.
func (r *JSONLCaptureRecorder) Record(observation CaptureObservation) error {
	if r == nil {
		return nil
	}

	fullHash := sha256.Sum256([]byte(observation.Content))
	paneIdentity := observation.SessionName + "\x00" + observation.PaneID

	r.mu.Lock()
	defer r.mu.Unlock()
	if previous, ok := r.lastHash[paneIdentity]; ok && previous == fullHash {
		return nil
	}

	content, truncated := boundRecordedContent(observation.Content)
	record := captureRecord{
		SchemaVersion:    captureRecordSchemaVersion,
		CapturedAt:       time.Now().UTC(),
		RunID:            r.runID,
		SessionKey:       r.opaqueKey("session", observation.SessionName),
		PaneKey:          r.opaqueKey("pane", observation.SessionName, observation.PaneID),
		Tool:             observation.Tool,
		Content:          content,
		ContentSHA256:    hex.EncodeToString(fullHash[:]),
		ContentTruncated: truncated,
		WeakLabel:        observation.Status,
		WeakLabelSource:  weakLabelSource,
	}
	line, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode capture record: %w", err)
	}
	line = append(line, '\n')
	if err := appendJSONLine(r.path, line); err != nil {
		return err
	}

	r.lastHash[paneIdentity] = fullHash
	return nil
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
	temp, err := os.CreateTemp(dir, ".identity-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("create capture recording identity key: %w", err)
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return nil, fmt.Errorf("secure capture recording identity key: %w", err)
	}
	if _, err := temp.Write(key); err != nil {
		_ = temp.Close()
		return nil, fmt.Errorf("write capture recording identity key: %w", err)
	}
	if err := temp.Close(); err != nil {
		return nil, fmt.Errorf("close capture recording identity key: %w", err)
	}

	if err := os.Link(tempPath, path); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("install capture recording identity key: %w", err)
		}
		return readIdentityKey(path)
	}
	return key, nil
}

func readIdentityKey(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("inspect capture recording identity key: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("capture recording identity key is not a regular file: %s", path)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("secure capture recording identity key: %w", err)
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

func appendJSONLine(path string, line []byte) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect capture recording: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("capture recording is not a regular file: %s", path)
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open capture recording: %w", err)
	}
	offset, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("seek capture recording: %w", err)
	}
	written, writeErr := file.Write(line)
	if writeErr == nil && written != len(line) {
		writeErr = io.ErrShortWrite
	}
	if writeErr != nil {
		truncateErr := file.Truncate(offset)
		closeErr := file.Close()
		return errors.Join(fmt.Errorf("append capture recording: %w", writeErr), truncateErr, closeErr)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close capture recording: %w", err)
	}
	return nil
}

func (r *JSONLCaptureRecorder) opaqueKey(namespace string, values ...string) string {
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

func boundRecordedContent(content string) (string, bool) {
	if len(content) <= maxRecordedContentBytes {
		return content, false
	}
	start := len(content) - maxRecordedContentBytes
	for start < len(content) && !utf8.RuneStart(content[start]) {
		start++
	}
	return content[start:], true
}
