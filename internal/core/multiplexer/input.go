package multiplexer

import (
	"fmt"
	"io"
	"strings"
)

// AttachStreams are connected directly to an interactive multiplexer client.
type AttachStreams struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// CaptureOptions controls pane capture behavior.
type CaptureOptions struct {
	JoinWrappedLines bool
	StartLine        *int
	EndLine          *int
}

// PasteOptions controls buffer paste behavior.
type PasteOptions struct {
	Bracketed bool
}

// NamedKey is one tmux named-key token.
type NamedKey string

// NewNamedKey validates a named key for safe argv transport.
func NewNamedKey(value string) (NamedKey, error) {
	if value == "" {
		return "", fmt.Errorf("named key is required")
	}
	if strings.ContainsRune(value, '\x00') {
		return "", fmt.Errorf("named key contains NUL")
	}
	return NamedKey(value), nil
}
