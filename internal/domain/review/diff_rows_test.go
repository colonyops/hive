package review

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const patch = `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -10,4 +10,5 @@ func f() {
 	one
-	two
+	deux
+	trois
 	four
@@ -40,2 +41,2 @@
 	forty
-	gone
`

func TestParseDiffRowsNumbersBothSides(t *testing.T) {
	rows := ParseDiffRows(patch)
	assert.Equal(t, []DiffRow{
		{Type: DiffRowContext, OldLine: 10, NewLine: 10, Text: "\tone"},
		{Type: DiffRowDeletion, OldLine: 11, Text: "\ttwo"},
		{Type: DiffRowAddition, NewLine: 11, Text: "\tdeux"},
		{Type: DiffRowAddition, NewLine: 12, Text: "\ttrois"},
		{Type: DiffRowContext, OldLine: 12, NewLine: 13, Text: "\tfour"},
		{Type: DiffRowContext, OldLine: 40, NewLine: 41, Text: "\tforty"},
		{Type: DiffRowDeletion, OldLine: 41, Text: "\tgone"},
	}, rows)
}

func TestDiffSpanQuote(t *testing.T) {
	rows := ParseDiffRows(patch)

	quote, ok := DiffSpanQuote(rows, DiffSideDeletions, 11, DiffSideAdditions, 12)
	assert.True(t, ok)
	assert.Equal(t, "-\ttwo\n+\tdeux\n+\ttrois", quote)

	quote, ok = DiffSpanQuote(rows, DiffSideAdditions, 13, DiffSideAdditions, 13)
	assert.True(t, ok)
	assert.Equal(t, " \tfour", quote)

	_, ok = DiffSpanQuote(rows, DiffSideAdditions, 20, DiffSideAdditions, 20)
	assert.False(t, ok, "a line between hunks is no row")

	_, ok = DiffSpanQuote(rows, DiffSideAdditions, 12, DiffSideAdditions, 11)
	assert.False(t, ok, "a span runs forward")
}
