package tui

import (
	"strings"
	"testing"

	"github.com/hay-kot/hive/internal/core/notify"
	"github.com/hay-kot/hive/internal/core/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToastView_View_empty(t *testing.T) {
	c := NewToastController()
	v := NewToastView(c)

	assert.Empty(t, v.View())
}

func TestToastView_View_renders_each_level(t *testing.T) {
	tests := []struct {
		level notify.Level
		icon  string
	}{
		{notify.LevelError, styles.IconNotifyError},
		{notify.LevelWarning, styles.IconNotifyWarning},
		{notify.LevelInfo, styles.IconNotifyInfo},
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			c := NewToastController()
			v := NewToastView(c)

			c.Push(notify.Notification{Level: tt.level, Message: "test msg"})

			out := v.View()
			require.NotEmpty(t, out)
			assert.Contains(t, out, tt.icon)
			assert.Contains(t, out, "test msg")
		})
	}
}

func TestToastView_View_stacks_multiple(t *testing.T) {
	c := NewToastController()
	v := NewToastView(c)

	c.Push(notify.Notification{Level: notify.LevelInfo, Message: "first"})
	c.Push(notify.Notification{Level: notify.LevelError, Message: "second"})

	out := v.View()
	firstIdx := strings.Index(out, "first")
	secondIdx := strings.Index(out, "second")

	require.NotEqual(t, -1, firstIdx)
	require.NotEqual(t, -1, secondIdx)
	// Oldest (first) should appear before newest (second) in the output.
	assert.Less(t, firstIdx, secondIdx)
}

func TestToastView_Overlay_empty_returns_background(t *testing.T) {
	c := NewToastController()
	v := NewToastView(c)

	bg := "background content"
	assert.Equal(t, bg, v.Overlay(bg, 80, 24))
}

func TestToastView_Overlay_positions_lower_right(t *testing.T) {
	c := NewToastController()
	v := NewToastView(c)

	c.Push(notify.Notification{Level: notify.LevelInfo, Message: "positioned"})

	width := 120
	height := 40

	// Build a proper background grid.
	row := strings.Repeat(" ", width)
	rows := make([]string, height)
	for i := range rows {
		rows[i] = row
	}
	bg := strings.Join(rows, "\n")

	out := v.Overlay(bg, width, height)

	// The toast content should be present in the composited output.
	assert.Contains(t, out, "positioned")

	// Toast should appear near the bottom — check that lines containing the
	// toast text are in the lower half of the output.
	lines := strings.Split(out, "\n")
	toastLine := -1
	for i, line := range lines {
		if strings.Contains(line, "positioned") {
			toastLine = i
			break
		}
	}
	require.NotEqual(t, -1, toastLine, "toast text not found in output lines")
	assert.Greater(t, toastLine, height/2, "toast should be in the lower half")
}
