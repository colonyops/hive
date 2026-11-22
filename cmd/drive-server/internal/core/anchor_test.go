package core_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
)

const planV1 = `# Plan

## Phase 1

We rotate every API token in one release.
Keep the old ones valid for 7 days.

## Phase 2

Clean up the flags.
`

func anchor(start, end int, quote string) core.ThreadAnchor {
	return core.ThreadAnchor{ThreadID: "t", StartLine: start, EndLine: end, Quote: quote}
}

func TestReanchor(t *testing.T) {
	tests := []struct {
		name string
		prev string
		next string
		in   core.ThreadAnchor
		want core.ThreadAnchor
	}{
		{
			name: "unchanged lines move down with an insert above",
			prev: planV1,
			next: "# Plan\n\nIntro line.\nAnother.\n" + planV1[len("# Plan\n"):],
			in:   anchor(5, 6, "We rotate every API token in one release. Keep the old ones valid"),
			want: anchor(8, 9, "We rotate every API token in one release. Keep the old ones valid"),
		},
		{
			name: "an edited line in the range is found by its quote",
			prev: planV1,
			next: "# Plan\n\n## Phase 1\n\nWe rotate every API token in two releases.\nKeep the old ones valid for 7 days.\n",
			in:   anchor(5, 6, "Keep the old ones valid for 7 days."),
			want: anchor(6, 6, "Keep the old ones valid for 7 days."),
		},
		{
			name: "markdown formatting in the source still matches the rendered quote",
			prev: planV1,
			next: "# Plan\n\nNew intro.\n\n## Phase 1\n\nWe rotate **every** API `token` in one\nrelease, honestly.\n",
			in:   anchor(5, 5, "We rotate every API token in one"),
			want: anchor(7, 7, "We rotate every API token in one"),
		},
		{
			name: "of two matches the one nearer the old place wins",
			prev: "a\nb\nTarget text\nc\nd\ne\n",
			next: "Target text\na\nb changed\nTarget text\nc\nd\ne\n",
			in:   anchor(3, 3, "Target text"),
			want: anchor(4, 4, "Target text"),
		},
		{
			name: "a whole-word match beats a match inside a word",
			prev: "x\none\n",
			next: "someone\nx\nchanged\none\n",
			in:   anchor(2, 2, "one"),
			want: anchor(4, 4, "one"),
		},
		{
			name: "a quote across list items ignores the bullets and numbers",
			prev: "intro\n\n1. first step\n2. second step\n",
			next: "intro changed\n\n1. first step\n2. second step!\n",
			in:   anchor(3, 4, "first step\nsecond step"),
			want: anchor(3, 4, "first step\nsecond step"),
		},
		{
			name: "a removed quote goes outdated and keeps its lines",
			prev: planV1,
			next: "# Plan\n\n## Phase 1\n\nSomething else entirely.\n",
			in:   anchor(6, 6, "Keep the old ones valid for 7 days."),
			want: core.ThreadAnchor{ThreadID: "t", StartLine: 6, EndLine: 6, Quote: "Keep the old ones valid for 7 days.", Outdated: true},
		},
		{
			name: "an outdated quote comes back when the text returns",
			prev: "# Plan\n\nSomething else.\n",
			next: "# Plan\n\nSomething else.\n\nKeep the old ones valid for 7 days.\n",
			in:   core.ThreadAnchor{ThreadID: "t", StartLine: 6, EndLine: 6, Quote: "Keep the old ones valid for 7 days.", Outdated: true},
			want: anchor(5, 5, "Keep the old ones valid for 7 days."),
		},
		{
			name: "an unchanged document keeps every anchor",
			prev: planV1,
			next: planV1,
			in:   anchor(10, 10, "Clean up the flags."),
			want: anchor(10, 10, "Clean up the flags."),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := core.Reanchor([]byte(tt.prev), []byte(tt.next), []core.ThreadAnchor{tt.in})
			assert.Equal(t, []core.ThreadAnchor{tt.want}, got)
		})
	}
}

func TestReanchorWithoutAnchorsIsNil(t *testing.T) {
	assert.Nil(t, core.Reanchor([]byte("a"), []byte("b"), nil))
}

func TestNormalizeQuote(t *testing.T) {
	assert.Equal(t, "keep the old ones", core.NormalizeQuote("  Keep the **old** ones! "))
	assert.Equal(t, "ünïcode 42", core.NormalizeQuote("Ünïcode — 42"))
	assert.Empty(t, core.NormalizeQuote("** -- **"))
}
