package review

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// SearchMode handles document search functionality.
type SearchMode struct {
	active       bool            // True when search mode is active
	input        textinput.Model // Search input field
	query        string          // Current search query
	matches      []int           // Line numbers of search matches (1-indexed)
	currentIndex int             // Current match index in matches
}

// NewSearchMode creates a new SearchMode instance.
func NewSearchMode() SearchMode {
	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.CharLimit = 100
	ti.KeyMap.Paste.SetEnabled(true)

	return SearchMode{
		active:       false,
		input:        ti,
		query:        "",
		matches:      nil,
		currentIndex: 0,
	}
}

// Activate enables search mode and focuses the input field.
func (s *SearchMode) Activate() {
	s.active = true
	s.input.Focus()
	s.input.SetValue("")
	s.query = ""
	s.matches = nil
	s.currentIndex = 0
}

// Deactivate disables search mode.
func (s *SearchMode) Deactivate() {
	s.active = false
	s.query = ""
	s.input.SetValue("")
	s.matches = nil
	s.currentIndex = 0
}

// Update handles key messages and updates the search input.
// Returns the updated SearchMode and any commands.
func (s SearchMode) Update(msg tea.Msg) (SearchMode, tea.Cmd) {
	if !s.active {
		return s, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}

	switch keyMsg.String() {
	case "enter": //nolint:goconst // Phase 1 refactor: will be extracted to constants in Phase 2
		// Execute search
		s.query = s.input.Value()
		// Note: actual search execution is handled by the caller
		// which will call Search() to get matches
		return s, nil
	case "esc":
		// Cancel search
		s.Deactivate()
		return s, nil
	default:
		// Update input
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		return s, cmd
	}
}

// View renders the search input bar.
// Returns empty string if search mode is not active.
func (s SearchMode) View() string {
	if !s.active {
		return ""
	}
	return s.input.View()
}

// Search searches through the provided document lines and returns matching line numbers.
// Lines should be from Document.RenderedLines.
// Returns 1-indexed line numbers of matches.
func (s *SearchMode) Search(lines []string) []int {
	s.matches = nil
	s.currentIndex = 0

	if s.query == "" {
		return nil
	}

	// Case-insensitive search
	queryLower := strings.ToLower(s.query)

	// Search through rendered lines (strip ANSI codes for searching)
	for i, line := range lines {
		// Strip ANSI codes before searching
		cleanLine := ansiStripPattern.ReplaceAllString(line, "")
		if strings.Contains(strings.ToLower(cleanLine), queryLower) {
			s.matches = append(s.matches, i+1) // Store 1-indexed line numbers
		}
	}

	return s.matches
}

// NextMatch advances to the next match and returns its line number.
// Returns 0 if there are no matches.
func (s *SearchMode) NextMatch() int {
	if len(s.matches) == 0 {
		return 0
	}

	s.currentIndex = (s.currentIndex + 1) % len(s.matches)
	return s.matches[s.currentIndex]
}

// PrevMatch goes back to the previous match and returns its line number.
// Returns 0 if there are no matches.
func (s *SearchMode) PrevMatch() int {
	if len(s.matches) == 0 {
		return 0
	}

	s.currentIndex = (s.currentIndex - 1 + len(s.matches)) % len(s.matches)
	return s.matches[s.currentIndex]
}

// CurrentMatch returns the line number of the current match.
// Returns 0 if there are no matches.
func (s *SearchMode) CurrentMatch() int {
	if len(s.matches) == 0 || s.currentIndex < 0 || s.currentIndex >= len(s.matches) {
		return 0
	}
	return s.matches[s.currentIndex]
}

// IsActive returns true if search mode is active.
func (s SearchMode) IsActive() bool {
	return s.active
}

// HasMatches returns true if there are search matches.
func (s SearchMode) HasMatches() bool {
	return len(s.matches) > 0
}

// MatchCount returns the number of search matches.
func (s SearchMode) MatchCount() int {
	return len(s.matches)
}

// MatchIndex returns the current match index (0-based).
func (s SearchMode) MatchIndex() int {
	return s.currentIndex
}

// Query returns the current search query.
func (s SearchMode) Query() string {
	return s.query
}

// Matches returns all search match line numbers (1-indexed).
func (s SearchMode) Matches() []int {
	return s.matches
}
