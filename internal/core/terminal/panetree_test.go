package terminal

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintPaneTree_NumericWindowSort(t *testing.T) {
	// Window 10 must follow window 9 (numeric), not 1 (lexicographic).
	panes := []PaneDetail{
		{TmuxSession: "sess", WindowIndex: "1", WindowName: "w1", PaneID: "%1"},
		{TmuxSession: "sess", WindowIndex: "10", WindowName: "w10", PaneID: "%10"},
		{TmuxSession: "sess", WindowIndex: "2", WindowName: "w2", PaneID: "%2"},
		{TmuxSession: "sess", WindowIndex: "9", WindowName: "w9", PaneID: "%9"},
	}
	var buf bytes.Buffer
	PrintPaneTree(&buf, panes)
	out := buf.String()

	wants := []string{"window 1 (w1)", "window 2 (w2)", "window 9 (w9)", "window 10 (w10)"}
	last := -1
	for _, w := range wants {
		idx := strings.Index(out, w)
		require.NotEqualf(t, -1, idx, "missing %q in output:\n%s", w, out)
		assert.Greaterf(t, idx, last, "expected %q after previous entry; output was:\n%s", w, out)
		last = idx
	}
}

func TestPrintPaneTree_PlaceholderForMissingFields(t *testing.T) {
	panes := []PaneDetail{
		{TmuxSession: "s", WindowIndex: "0", WindowName: "wn", PaneID: "%0"},
	}
	var buf bytes.Buffer
	PrintPaneTree(&buf, panes)
	out := buf.String()

	assert.Contains(t, out, "hive:(none)")
	assert.Contains(t, out, "tool:?")
	assert.Contains(t, out, "pid:-")
	assert.Contains(t, out, "fg:-")
}

func TestPrintPaneTree_MultiSessionAlphabetical(t *testing.T) {
	panes := []PaneDetail{
		{TmuxSession: "zebra", WindowIndex: "0", PaneID: "%0"},
		{TmuxSession: "alpha", WindowIndex: "0", PaneID: "%1"},
	}
	var buf bytes.Buffer
	PrintPaneTree(&buf, panes)
	out := buf.String()

	alphaIdx := strings.Index(out, "alpha")
	zebraIdx := strings.Index(out, "zebra")
	require.NotEqual(t, -1, alphaIdx)
	require.NotEqual(t, -1, zebraIdx)
	assert.Less(t, alphaIdx, zebraIdx, "sessions should be alphabetical")
}

func TestPrintPaneTree_PopulatedFields(t *testing.T) {
	panes := []PaneDetail{
		{
			TmuxSession: "s", WindowIndex: "0", WindowName: "agent", PaneID: "%0",
			PanePID: 1234, FgPID: 5678, HiveSession: "myslug", Tool: "claude",
		},
	}
	var buf bytes.Buffer
	PrintPaneTree(&buf, panes)
	out := buf.String()

	assert.Contains(t, out, "hive:myslug")
	assert.Contains(t, out, "tool:claude")
	assert.Contains(t, out, "pid:1234")
	assert.Contains(t, out, "fg:5678")
}

func TestPrintPaneTree_EmptyInput(t *testing.T) {
	var buf bytes.Buffer
	PrintPaneTree(&buf, nil)
	assert.Empty(t, buf.String())
}

func TestPrintPaneTree_TreeConnectors(t *testing.T) {
	panes := []PaneDetail{
		{TmuxSession: "s", WindowIndex: "0", PaneID: "%0"},
		{TmuxSession: "s", WindowIndex: "0", PaneID: "%1"},
		{TmuxSession: "s", WindowIndex: "1", PaneID: "%2"},
	}
	var buf bytes.Buffer
	PrintPaneTree(&buf, panes)
	out := buf.String()

	// First window of two: middle connector.
	assert.Contains(t, out, "├─ window 0")
	// Last window: closing connector.
	assert.Contains(t, out, "└─ window 1")
}

func TestWindowIndexLess(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"1", "2", true},
		{"2", "10", true},  // numeric, not lexicographic
		{"10", "2", false}, // numeric, not lexicographic
		{"a", "b", true},
		{"a", "1", false}, // numeric beats string ("1" parses, "a" doesn't → falls to lex; "1"<"a")
	}
	for _, tt := range tests {
		t.Run(tt.a+"<"+tt.b, func(t *testing.T) {
			assert.Equal(t, tt.want, windowIndexLess(tt.a, tt.b))
		})
	}
}
