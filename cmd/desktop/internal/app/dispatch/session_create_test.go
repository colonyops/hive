package dispatch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSessionProgressKeepsTheLastLineAndBoundsTheTail(t *testing.T) {
	progress := &sessionProgress{}
	for i := range maxProgressTailLines + 5 {
		_, _ = progress.Write([]byte("step " + string(rune('a'+i%26)) + "\n"))
	}
	_, _ = progress.Write([]byte("Cloning repository...\n"))

	assert.Equal(t, "Cloning repository...", progress.LastLine())
	tail := progress.Tail()
	assert.LessOrEqual(t, len(tail), maxProgressTail)
	assert.Contains(t, tail, "Cloning repository...")
	assert.NotContains(t, tail, "step a", "the oldest lines fall out of a bounded tail")
}

// hive writes a step line per Fprintf, but a hook's output arrives in
// whatever chunks the pipe delivers, so a write is not a line.
func TestSessionProgressSplitsPartialWritesIntoLines(t *testing.T) {
	progress := &sessionProgress{}
	_, _ = progress.Write([]byte("Executing rules...\nhook: "))
	_, _ = progress.Write([]byte("command not found\n\n"))

	assert.Equal(t, "hook: command not found", progress.LastLine())
	assert.Equal(t, "Executing rules...\nhook: command not found", progress.Tail())
}
