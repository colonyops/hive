package core

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/aymanbagabas/go-udiff"
)

// Reanchor moves each anchor from prev to next. A range whose lines are all
// unchanged moves with them. Otherwise the quote is searched for in next, and
// the match nearest the old position wins. An anchor with no match is marked
// Outdated and keeps its old lines and quote; it is searched for again on
// every push and comes back when its quote does.
func Reanchor(prev, next []byte, anchors []ThreadAnchor) []ThreadAnchor {
	if len(anchors) == 0 {
		return nil
	}
	lines := mapLines(string(prev), string(next))
	var source *normalized
	out := make([]ThreadAnchor, len(anchors))
	for i, a := range anchors {
		hint := a.StartLine
		if !a.Outdated {
			if start, end, ok := lines.moveRange(a.StartLine, a.EndLine); ok {
				a.StartLine, a.EndLine = start, end
				out[i] = a
				continue
			}
			if mapped, ok := lines.firstMapped(a.StartLine, a.EndLine); ok {
				hint = mapped
			}
		}
		if source == nil {
			n := normalizeSource(string(next))
			source = &n
		}
		if start, end, ok := source.find(NormalizeQuote(a.Quote), hint); ok {
			a.StartLine, a.EndLine, a.Outdated = start, end, false
		} else {
			a.Outdated = true
		}
		out[i] = a
	}
	return out
}

// lineMap maps a 1-based line of the old text to its line in the new text,
// or 0 when the line was changed or removed.
type lineMap []int

func mapLines(prev, next string) lineMap {
	starts := lineStarts(prev)
	m := make(lineMap, len(starts)+1)
	lineAt := func(off int) int {
		if off >= len(prev) {
			return len(starts)
		}
		return sort.Search(len(starts), func(i int) bool { return starts[i] > off }) - 1
	}
	shift, cursor := 0, 0
	for _, e := range udiff.Lines(prev, next) {
		first, last := lineAt(e.Start), lineAt(e.End)
		for l := cursor; l < first; l++ {
			m[l+1] = l + 1 + shift
		}
		shift += countLines(e.New) - (last - first)
		cursor = last
	}
	for l := cursor; l < len(starts); l++ {
		m[l+1] = l + 1 + shift
	}
	return m
}

func (m lineMap) at(line int) int {
	if line < 1 || line >= len(m) {
		return 0
	}
	return m[line]
}

// moveRange is the range's new lines when every line survived unchanged and
// stayed together.
func (m lineMap) moveRange(start, end int) (int, int, bool) {
	first := m.at(start)
	if first == 0 {
		return 0, 0, false
	}
	for l := start + 1; l <= end; l++ {
		if m.at(l) != first+(l-start) {
			return 0, 0, false
		}
	}
	return first, first + (end - start), true
}

func (m lineMap) firstMapped(start, end int) (int, bool) {
	for l := start; l <= end; l++ {
		if to := m.at(l); to != 0 {
			return to - (l - start), true
		}
	}
	return 0, false
}

func lineStarts(s string) []int {
	if s == "" {
		return nil
	}
	starts := []int{0}
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

func countLines(s string) int {
	n := strings.Count(s, "\n")
	if s != "" && !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

// NormalizeQuote lowercases s and keeps its letters and digits, with every run
// of anything else collapsed to one space. A quote is rendered text and the
// body is markdown source, so "keep the **old** ones" must match "keep the
// old ones".
func NormalizeQuote(s string) string {
	var b strings.Builder
	appendNormalized(&b, s, nil, 0)
	return strings.TrimSpace(b.String())
}

// normalized is markdown source run through the NormalizeQuote rule, with
// the 1-based source line of every output byte.
type normalized struct {
	text   string
	lineOf []int
}

// blockPrefix is the markup that starts a rendered block but is not part of
// its text: quote markers, list bullets and numbers, task boxes, headings.
// A selection across list items carries no "1." or "-".
var blockPrefix = regexp.MustCompile(`^[ \t]*(?:>[ \t]*)*(?:[-*+][ \t]+|\d+[.)][ \t]+|#{1,6}[ \t]+)?(?:\[[ xX]\][ \t]+)?`)

func normalizeSource(src string) normalized {
	var b strings.Builder
	var lineOf []int
	for i, line := range strings.Split(src, "\n") {
		line = line[len(blockPrefix.FindString(line)):]
		lineOf = appendNormalized(&b, line, lineOf, i+1)
		if b.Len() > 0 && !strings.HasSuffix(b.String(), " ") {
			b.WriteByte(' ')
			lineOf = append(lineOf, i+1)
		}
	}
	return normalized{text: b.String(), lineOf: lineOf}
}

// appendNormalized writes s to b under the NormalizeQuote rule. When line is
// above zero it also records line in lineOf for every byte written.
func appendNormalized(b *strings.Builder, s string, lineOf []int, line int) []int {
	for _, r := range s {
		written := 0
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			r = unicode.ToLower(r)
			b.WriteRune(r)
			written = utf8.RuneLen(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), " "):
			b.WriteByte(' ')
			written = 1
		}
		for ; line > 0 && written > 0; written-- {
			lineOf = append(lineOf, line)
		}
	}
	return lineOf
}

// find returns the lines of the match of quote nearest hint. Whole-word
// matches win over matches inside a word, so "one" does not land in
// "someone" when the word itself is there.
func (n normalized) find(quote string, hint int) (int, int, bool) {
	if quote == "" {
		return 0, 0, false
	}
	for _, padded := range []bool{true, false} {
		best, bestStart, bestEnd := math.MaxInt, 0, 0
		for i := 0; ; {
			j := strings.Index(n.text[i:], quote)
			if j < 0 {
				break
			}
			at := i + j
			i = at + 1
			if padded && (!wordEdge(n.text, at-1) || !wordEdge(n.text, at+len(quote))) {
				continue
			}
			start, end := n.lineOf[at], n.lineOf[at+len(quote)-1]
			if d := abs(start - hint); d < best {
				best, bestStart, bestEnd = d, start, end
			}
		}
		if best != math.MaxInt {
			return bestStart, bestEnd, true
		}
	}
	return 0, 0, false
}

func wordEdge(s string, i int) bool { return i < 0 || i >= len(s) || s[i] == ' ' }

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
