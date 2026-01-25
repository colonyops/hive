// Package history defines command history domain types and interfaces.
package history

import "time"

// Entry represents a recorded command execution.
type Entry struct {
	ID        string    `json:"id"`
	Command   string    `json:"command"`
	Args      []string  `json:"args"`
	ExitCode  int       `json:"exit_code"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// Failed returns true if the command exited with a non-zero exit code.
func (e *Entry) Failed() bool {
	return e.ExitCode != 0
}
