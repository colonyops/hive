package timer

import (
	"fmt"
	"time"
)

const (
	// MinDuration is the smallest --duration value accepted by `hive timer`.
	MinDuration = 1 * time.Second
	// MaxDuration is the largest --duration value accepted by `hive timer`.
	// A 4-hour cap keeps the bookkeeping table tidy while still covering
	// realistic "check on me later today" use cases.
	MaxDuration = 4 * time.Hour

	// MinPromptSize is the smallest --prompt size in bytes (excludes empty).
	MinPromptSize = 1
	// MaxPromptSize is the largest --prompt size in bytes. Keeps a single
	// tmux send-keys invocation reasonable.
	MaxPromptSize = 8192
)

// ParseDuration parses s with time.ParseDuration and enforces the
// [MinDuration, MaxDuration] bounds. The returned error is descriptive and
// suitable for direct CLI output.
func ParseDuration(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", s, err)
	}
	if d < MinDuration {
		return 0, fmt.Errorf("duration %s is below minimum %s", d, MinDuration)
	}
	if d > MaxDuration {
		return 0, fmt.Errorf("duration %s exceeds maximum %s", d, MaxDuration)
	}
	return d, nil
}

// ValidatePrompt enforces [MinPromptSize, MaxPromptSize] on the byte length
// of the prompt string. Empty prompts and prompts > MaxPromptSize bytes are
// rejected with a descriptive error.
func ValidatePrompt(p string) error {
	if len(p) < MinPromptSize {
		return fmt.Errorf("prompt is empty (minimum %d byte)", MinPromptSize)
	}
	if len(p) > MaxPromptSize {
		return fmt.Errorf("prompt is %d bytes; maximum is %d", len(p), MaxPromptSize)
	}
	return nil
}
