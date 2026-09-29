package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

// makeRepos returns n repo strings "repo-0".."repo-n-1".
func makeRepos(n int) []string {
	repos := make([]string, n)
	for i := range repos {
		repos[i] = "repo-" + string(rune('0'+i%10))
		if i >= 10 {
			repos[i] = "repo-" + string(rune('a'+i-10))
		}
	}
	return repos
}

// repoPickerNavigate applies n "down" keypresses to the picker.
func repoPickerNavigate(p *RepoPicker, n int) *RepoPicker {
	for range n {
		p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	}
	return p
}

// TestRepoPicker_CursorVisibleInView verifies that the selection indicator (▸)
// is always present in the rendered view regardless of list length and cursor position.
func TestRepoPicker_CursorVisibleInView(t *testing.T) {
	// height=30 → maxVisible = min(len, max(30/3,5)) = min(len, 10)
	// With 15 repos, navigating past index 9 exposes the bug.
	repos := makeRepos(15)
	height := 30
	p := NewRepoPicker(repos, "", 120, height)

	// Navigate to the last item (index 14), which is beyond maxVisible (10).
	for range 14 {
		p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	}

	assert.Equal(t, 14, p.cursor, "cursor should be at last item")

	view := p.View()
	assert.Contains(t, view, "▸",
		"selection indicator must be visible when cursor is beyond the initial window; got view:\n%s", view)
}

// TestRepoPicker_SelectedItemIsVisible checks that the selected item's text appears in the view.
func TestRepoPicker_SelectedItemIsVisible(t *testing.T) {
	repos := []string{
		"alpha", "beta", "gamma", "delta", "epsilon",
		"zeta", "eta", "theta", "iota", "kappa",
		"lambda", "mu", "nu", "xi", "omicron",
	}
	// height=30 → maxVisible=10; navigate to item 12 ("nu")
	p := NewRepoPicker(repos, "", 120, 30)
	p = repoPickerNavigate(p, 12)

	assert.Equal(t, 12, p.cursor)

	view := p.View()
	assert.Contains(t, view, "nu",
		"selected item text must appear in the rendered view; got:\n%s", view)
}

// TestRepoPicker_NavigationBounds ensures cursor never goes below 0 or above len-1.
func TestRepoPicker_NavigationBounds(t *testing.T) {
	repos := makeRepos(3)
	p := NewRepoPicker(repos, "", 80, 30)

	// Pressing up at top should stay at 0.
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	assert.Equal(t, 0, p.cursor, "cursor should not go below 0")

	// Navigate to end.
	p = repoPickerNavigate(p, 10) // more than len
	assert.Equal(t, 2, p.cursor, "cursor should not exceed len(filtered)-1")
}

// TestRepoPicker_ScrollOffset_TracksWithCursor tests that scrollOffset advances when
// cursor moves down past the visible window and retreats when cursor moves back up.
func TestRepoPicker_ScrollOffset_TracksWithCursor(t *testing.T) {
	repos := makeRepos(15)
	p := NewRepoPicker(repos, "", 120, 30) // maxVisible=10

	// Navigate just at the edge — cursor at 9 (last visible without scroll).
	p = repoPickerNavigate(p, 9)
	assert.Equal(t, 9, p.cursor)
	view := p.View()
	assert.Contains(t, view, "▸", "cursor at 9 should still be visible")

	// Navigate one more — cursor at 10 (first item beyond initial window).
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	assert.Equal(t, 10, p.cursor)
	view = p.View()
	assert.Contains(t, view, "▸",
		"cursor at 10 (beyond maxVisible window) must still show selection indicator")

	// Navigate back — cursor at 9 again, should still show indicator.
	p, _ = p.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	assert.Equal(t, 9, p.cursor)
	view = p.View()
	assert.Contains(t, view, "▸", "after scrolling back, cursor at 9 should be visible")
}
