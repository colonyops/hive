// Package promptfile moves oversized agent prompts out of process arguments.
package promptfile

import (
	"errors"
	"fmt"
	"os"
)

// ThresholdBytes is the largest prompt Hive passes directly as a process
// argument. Larger prompts use a temporary file so they stay below platform
// argument limits.
const ThresholdBytes = 32 * 1024

// Prepared is the prompt text passed to an agent and the optional temporary
// file containing the original prompt.
type Prepared struct {
	Prompt string
	Path   string
}

// Prepare leaves a small prompt unchanged. It writes an oversized prompt to a
// private temporary file and replaces it with instructions to read that file.
func Prepare(prompt string) (Prepared, error) {
	if len(prompt) <= ThresholdBytes {
		return Prepared{Prompt: prompt}, nil
	}

	file, err := os.CreateTemp("", "hive-prompt-*.md")
	if err != nil {
		return Prepared{}, fmt.Errorf("create prompt temp file: %w", err)
	}
	path := file.Name()
	if _, err := file.WriteString(prompt); err != nil {
		cleanupErr := errors.Join(file.Close(), os.Remove(path))
		return Prepared{}, errors.Join(fmt.Errorf("write prompt temp file: %w", err), cleanupErr)
	}
	if err := file.Close(); err != nil {
		return Prepared{}, errors.Join(fmt.Errorf("close prompt temp file: %w", err), os.Remove(path))
	}

	return Prepared{
		Prompt: fmt.Sprintf("The full prompt was too large to pass directly. Read it from `%s` before starting, then delete the file.", path),
		Path:   path,
	}, nil
}

// Remove deletes the temporary file, if Prepare created one.
func (p Prepared) Remove() error {
	if p.Path == "" {
		return nil
	}
	if err := os.Remove(p.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove prompt temp file: %w", err)
	}
	return nil
}
