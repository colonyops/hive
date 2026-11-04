package review

import (
	"regexp"
	"strconv"
	"strings"
)

// DiffRowType is what one row of a unified diff shows.
type DiffRowType string

const (
	DiffRowContext  DiffRowType = "context"
	DiffRowAddition DiffRowType = "addition"
	DiffRowDeletion DiffRowType = "deletion"
)

// DiffRow is one row of a file's diff. OldLine is 0 on an added row, NewLine
// on a removed one.
type DiffRow struct {
	Type    DiffRowType
	OldLine int
	NewLine int
	Text    string
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// ParseDiffRows reads one file's patch text into its rows, in the order a
// unified diff shows them. It matches diffRows in the desktop's
// lib/diffReview.ts, so a span quoted here reads the same as one quoted there.
func ParseDiffRows(patch string) []DiffRow {
	var rows []DiffRow
	oldLine, newLine := 0, 0
	inHunk := false
	for line := range strings.SplitSeq(patch, "\n") {
		if m := hunkHeader.FindStringSubmatch(line); m != nil {
			oldLine, _ = strconv.Atoi(m[1])
			newLine, _ = strconv.Atoi(m[2])
			inHunk = true
			continue
		}
		if !inHunk {
			continue
		}
		switch {
		case strings.HasPrefix(line, "+"):
			rows = append(rows, DiffRow{Type: DiffRowAddition, NewLine: newLine, Text: line[1:]})
			newLine++
		case strings.HasPrefix(line, "-"):
			rows = append(rows, DiffRow{Type: DiffRowDeletion, OldLine: oldLine, Text: line[1:]})
			oldLine++
		case strings.HasPrefix(line, " "):
			rows = append(rows, DiffRow{Type: DiffRowContext, OldLine: oldLine, NewLine: newLine, Text: line[1:]})
			oldLine++
			newLine++
		}
	}
	return rows
}

func rowIndex(rows []DiffRow, line int, side DiffSide) int {
	for i, row := range rows {
		n := row.NewLine
		if side == DiffSideDeletions {
			n = row.OldLine
		}
		if n == line {
			return i
		}
	}
	return -1
}

// DiffSpanQuote returns the rows from one line to another as patch text, and
// false when either line is not a row of the diff or the span runs backwards.
func DiffSpanQuote(rows []DiffRow, startSide DiffSide, startLine int, endSide DiffSide, endLine int) (string, bool) {
	from := rowIndex(rows, startLine, startSide)
	to := rowIndex(rows, endLine, endSide)
	if from < 0 || to < 0 || to < from {
		return "", false
	}
	lines := make([]string, 0, to-from+1)
	for _, row := range rows[from : to+1] {
		var prefix string
		switch row.Type {
		case DiffRowAddition:
			prefix = "+"
		case DiffRowDeletion:
			prefix = "-"
		case DiffRowContext:
			prefix = " "
		}
		lines = append(lines, prefix+row.Text)
	}
	return strings.Join(lines, "\n"), true
}
