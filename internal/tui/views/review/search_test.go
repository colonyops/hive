package review

import (
	"testing"

	"github.com/colonyops/hive/internal/tui/testutil"
)

func TestSearchMode_View(t *testing.T) {
	// Create SearchMode with empty search input
	searchMode := NewSearchMode()
	searchMode.Activate()

	output := searchMode.View()
	output = testutil.StripANSI(output)

	testutil.RequireGolden(t, output)
}

func TestSearchMode_ViewWithQuery(t *testing.T) {
	// Create SearchMode with a query
	searchMode := NewSearchMode()
	searchMode.Activate()
	searchMode.input.SetValue("test query")

	output := searchMode.View()
	output = testutil.StripANSI(output)

	testutil.RequireGolden(t, output)
}

func TestSearchMode_Search(t *testing.T) {
	tests := []struct {
		name      string
		lines     []string
		query     string
		wantCount int
		wantLines []int
	}{
		{
			name:      "empty query returns no matches",
			lines:     []string{"line 1", "line 2", "line 3"},
			query:     "",
			wantCount: 0,
			wantLines: nil,
		},
		{
			name:      "case insensitive matching",
			lines:     []string{"Line 1", "line 2", "LINE 3"},
			query:     "line",
			wantCount: 3,
			wantLines: []int{1, 2, 3},
		},
		{
			name:      "partial match works",
			lines:     []string{"testing", "test", "contest"},
			query:     "test",
			wantCount: 3,
			wantLines: []int{1, 2, 3},
		},
		{
			name:      "no matches returns empty",
			lines:     []string{"foo", "bar", "baz"},
			query:     "qux",
			wantCount: 0,
			wantLines: nil,
		},
		{
			name: "strips ANSI codes before matching",
			lines: []string{
				"\x1b[31mred text\x1b[0m",
				"plain text",
				"\x1b[1;34mblue text\x1b[0m",
			},
			query:     "text",
			wantCount: 3,
			wantLines: []int{1, 2, 3},
		},
		{
			name:      "multiple matches on same line",
			lines:     []string{"test test test"},
			query:     "test",
			wantCount: 1,
			wantLines: []int{1},
		},
		{
			name:      "returns 1-indexed line numbers",
			lines:     []string{"no", "match", "here", "match"},
			query:     "match",
			wantCount: 2,
			wantLines: []int{2, 4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSearchMode()
			s.query = tt.query

			got := s.Search(tt.lines)

			if len(got) != tt.wantCount {
				t.Errorf("Search() returned %d matches, want %d", len(got), tt.wantCount)
			}

			if !equalIntSlices(got, tt.wantLines) {
				t.Errorf("Search() = %v, want %v", got, tt.wantLines)
			}
		})
	}
}

func TestSearchMode_NextMatch(t *testing.T) {
	tests := []struct {
		name      string
		matches   []int
		callCount int
		want      []int // expected result after each NextMatch call
	}{
		{
			name:      "no matches returns 0",
			matches:   []int{},
			callCount: 3,
			want:      []int{0, 0, 0},
		},
		{
			name:      "single match always returns same",
			matches:   []int{5},
			callCount: 3,
			want:      []int{5, 5, 5},
		},
		{
			name:      "wraps from last to first",
			matches:   []int{1, 3, 5},
			callCount: 4,
			want:      []int{3, 5, 1, 3}, // 1->3->5->1(wrap)->3
		},
		{
			name:      "two matches alternate correctly",
			matches:   []int{2, 4},
			callCount: 5,
			want:      []int{4, 2, 4, 2, 4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSearchMode()
			s.matches = tt.matches
			s.currentIndex = 0

			var got []int
			for i := 0; i < tt.callCount; i++ {
				result := s.NextMatch()
				got = append(got, result)
			}

			if !equalIntSlices(got, tt.want) {
				t.Errorf("NextMatch() calls = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSearchMode_PrevMatch(t *testing.T) {
	tests := []struct {
		name      string
		matches   []int
		callCount int
		want      []int // expected result after each PrevMatch call
	}{
		{
			name:      "no matches returns 0",
			matches:   []int{},
			callCount: 3,
			want:      []int{0, 0, 0},
		},
		{
			name:      "single match always returns same",
			matches:   []int{5},
			callCount: 3,
			want:      []int{5, 5, 5},
		},
		{
			name:      "wraps from first to last",
			matches:   []int{1, 3, 5},
			callCount: 4,
			want:      []int{5, 3, 1, 5}, // 1->5(wrap)->3->1->5(wrap)
		},
		{
			name:      "two matches alternate correctly",
			matches:   []int{2, 4},
			callCount: 5,
			want:      []int{4, 2, 4, 2, 4}, // Start at 0, prev wraps to 1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSearchMode()
			s.matches = tt.matches
			s.currentIndex = 0

			var got []int
			for i := 0; i < tt.callCount; i++ {
				result := s.PrevMatch()
				got = append(got, result)
			}

			if !equalIntSlices(got, tt.want) {
				t.Errorf("PrevMatch() calls = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSearchMode_CurrentMatch(t *testing.T) {
	tests := []struct {
		name         string
		matches      []int
		currentIndex int
		want         int
	}{
		{
			name:         "no matches returns 0",
			matches:      []int{},
			currentIndex: 0,
			want:         0,
		},
		{
			name:         "valid index returns match",
			matches:      []int{1, 3, 5, 7},
			currentIndex: 2,
			want:         5,
		},
		{
			name:         "first match",
			matches:      []int{10, 20, 30},
			currentIndex: 0,
			want:         10,
		},
		{
			name:         "last match",
			matches:      []int{10, 20, 30},
			currentIndex: 2,
			want:         30,
		},
		{
			name:         "negative index returns 0",
			matches:      []int{1, 2, 3},
			currentIndex: -1,
			want:         0,
		},
		{
			name:         "index out of bounds returns 0",
			matches:      []int{1, 2, 3},
			currentIndex: 10,
			want:         0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSearchMode()
			s.matches = tt.matches
			s.currentIndex = tt.currentIndex

			got := s.CurrentMatch()
			if got != tt.want {
				t.Errorf("CurrentMatch() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSearchMode_NavigationSequence(t *testing.T) {
	// Test a realistic sequence of Next/Prev calls
	s := NewSearchMode()
	s.query = "test"

	lines := []string{
		"line 1",
		"test line 2",
		"line 3",
		"test line 4",
		"test line 5",
	}

	matches := s.Search(lines)
	if len(matches) != 3 {
		t.Fatalf("Expected 3 matches, got %d", len(matches))
	}

	// Start at match 1 (line 2)
	if s.CurrentMatch() != 2 {
		t.Errorf("Initial match = %d, want 2", s.CurrentMatch())
	}

	// Next -> match 2 (line 4)
	if got := s.NextMatch(); got != 4 {
		t.Errorf("After NextMatch() = %d, want 4", got)
	}

	// Next -> match 3 (line 5)
	if got := s.NextMatch(); got != 5 {
		t.Errorf("After NextMatch() = %d, want 5", got)
	}

	// Next -> wrap to match 1 (line 2)
	if got := s.NextMatch(); got != 2 {
		t.Errorf("After NextMatch() (wrap) = %d, want 2", got)
	}

	// Prev -> wrap to match 3 (line 5)
	if got := s.PrevMatch(); got != 5 {
		t.Errorf("After PrevMatch() (wrap) = %d, want 5", got)
	}

	// Prev -> match 2 (line 4)
	if got := s.PrevMatch(); got != 4 {
		t.Errorf("After PrevMatch() = %d, want 4", got)
	}
}

// Helper function to compare int slices
func equalIntSlices(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
