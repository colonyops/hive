package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOutputModal_NewOutputModal(t *testing.T) {
	m := NewOutputModal("Test Title")

	assert.Equal(t, "Test Title", m.title)
	assert.True(t, m.running)
	assert.Empty(t, m.lines)
	assert.Equal(t, outputModalMaxLines, m.maxLines)
}

func TestOutputModal_AddLine(t *testing.T) {
	t.Run("adds single line", func(t *testing.T) {
		m := NewOutputModal("Test")
		m.AddLine("line 1")

		assert.Equal(t, []string{"line 1"}, m.lines)
	})

	t.Run("splits multiline input", func(t *testing.T) {
		m := NewOutputModal("Test")
		m.AddLine("line 1\nline 2\nline 3")

		assert.Equal(t, []string{"line 1", "line 2", "line 3"}, m.lines)
	})

	t.Run("trims trailing newline", func(t *testing.T) {
		m := NewOutputModal("Test")
		m.AddLine("line 1\n")

		assert.Equal(t, []string{"line 1"}, m.lines)
	})

	t.Run("respects maxLines limit", func(t *testing.T) {
		m := NewOutputModal("Test")
		m.maxLines = 3

		for i := 0; i < 10; i++ {
			m.AddLine("line")
		}

		assert.Len(t, m.lines, 3)
	})
}

func TestOutputModal_SetComplete(t *testing.T) {
	t.Run("marks as complete without error", func(t *testing.T) {
		m := NewOutputModal("Test")
		assert.True(t, m.IsRunning())

		m.SetComplete(nil)

		assert.False(t, m.IsRunning())
		assert.NoError(t, m.err)
	})

	t.Run("marks as complete with error", func(t *testing.T) {
		m := NewOutputModal("Test")
		err := errors.New("test error")

		m.SetComplete(err)

		assert.False(t, m.IsRunning())
		assert.Equal(t, err, m.err)
	})
}

func TestOutputModal_Overlay(t *testing.T) {
	t.Run("renders with running state", func(t *testing.T) {
		m := NewOutputModal("Running Task")
		m.AddLine("output line")

		result := m.Overlay("background", 80, 24)

		assert.Contains(t, result, "Running Task")
		assert.Contains(t, result, "output line")
		assert.Contains(t, result, "Running...")
		assert.Contains(t, result, "[esc] cancel")
	})

	t.Run("renders complete state", func(t *testing.T) {
		m := NewOutputModal("Completed Task")
		m.SetComplete(nil)

		result := m.Overlay("background", 80, 24)

		assert.Contains(t, result, "Complete")
		assert.Contains(t, result, "[enter/esc] close")
	})

	t.Run("renders error state", func(t *testing.T) {
		m := NewOutputModal("Failed Task")
		m.SetComplete(errors.New("something went wrong"))

		result := m.Overlay("background", 80, 24)

		assert.Contains(t, result, "Error")
		assert.Contains(t, result, "something went wrong")
	})

	t.Run("truncates long lines", func(t *testing.T) {
		m := NewOutputModal("Test")
		longLine := strings.Repeat("x", 200)
		m.AddLine(longLine)

		result := m.Overlay("background", 80, 24)

		assert.Contains(t, result, "...")
		assert.NotContains(t, result, longLine)
	})
}
