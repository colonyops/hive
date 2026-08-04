package assess

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Region resolution is the highest-risk code in this package (the plan's own
// success criteria call it out): this file validates each region primitive
// against realistic tmux `capture-pane -J` output, independent of the
// engine/rule layer above it.

func readRegionFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "regions", name))
	require.NoError(t, err, "reading region fixture %s", name)
	return string(data)
}

func TestComputeRegions_PromptBoxBorders(t *testing.T) {
	content := readRegionFixture(t, "basic-box.txt")
	r := computeRegions(content)

	assert.True(t, r.hasPromptBox(), "real ╭/│/╰ borders must be detected as a prompt box")
	assert.Equal(t, ">", r.promptBoxBody())
	assert.Equal(t, "Previous turn output here.", r.abovePromptBox(),
		"abovePromptBox must return only the last contiguous block, not everything above the box")
	assert.Equal(t, "╭──╮\n│ > │\n╰──╯", r.bottomLines(3))
}

func TestComputeRegions_InteriorBlankLinesPreserved(t *testing.T) {
	content := readRegionFixture(t, "interior-blank-lines.txt")
	r := computeRegions(content)

	require.True(t, r.hasPromptBox())
	assert.Equal(t, "Bash command\n\ngo test ./internal/example/...", r.promptBoxBody(),
		"a blank line inside the box body must survive, not be dropped like getLastNonEmptyLines did")
}

func TestComputeRegions_WrappedLinePreservedWhole(t *testing.T) {
	content := readRegionFixture(t, "wrapped-line.txt")
	r := computeRegions(content)

	require.True(t, r.hasPromptBox())
	want := "This is a very long single logical line representing a wrapped terminal row that tmux capture-pane -J already joined into one piece"
	assert.Equal(t, want, r.promptBoxBody(),
		"capture-pane -J already joins wrapped physical lines; region resolution must not re-split or truncate them")
}

func TestComputeRegions_NoBoxFallsBackToLastContiguousBlock(t *testing.T) {
	content := readRegionFixture(t, "no-box-fullscreen.txt")
	r := computeRegions(content)

	assert.False(t, r.hasPromptBox())
	assert.Empty(t, r.promptBoxBody())
	// "OLD OUTPUT" sits above a blank-line boundary and must be excluded: the
	// no-box fallback is the last contiguous block of the viewport, not the
	// whole viewport — the same restriction that keeps stale scrollback out
	// of rule scope when a box IS present also applies here.
	assert.Equal(t, "HEADER\nline one\nline two\nline three", r.abovePromptBox(),
		"fullscreen TUIs with no box (vim, clipped panes) must fall back to the last contiguous block, not the whole viewport")
	assert.NotContains(t, r.abovePromptBox(), "OLD OUTPUT")
}

func TestComputeRegions_ClippedTopBorderDegradesWithoutMisparsing(t *testing.T) {
	content := readRegionFixture(t, "clipped-borders.txt")
	r := computeRegions(content)

	assert.False(t, r.hasPromptBox(), "a ╰ with no matching ╭ (top scrolled off a short pane) must not be misread as a box")
	assert.Empty(t, r.promptBoxBody())
	assert.NotPanics(t, func() { r.abovePromptBox() })
}

func TestComputeRegions_EmptyContent(t *testing.T) {
	r := computeRegions("")
	assert.False(t, r.hasPromptBox())
	assert.Empty(t, r.abovePromptBox())
	assert.Empty(t, r.promptBoxBody())
	assert.Empty(t, r.bottomLines(15))
	assert.Empty(t, r.afterLastRule())
}

func TestNormalizeContent_NBSPFoldedToSpace(t *testing.T) {
	// A literal NBSP (U+00A0) inside the prompt box, as some terminal
	// renderers emit for alignment padding. Region resolution runs on
	// already-normalized content, so this must never surface as a raw NBSP
	// byte in any resolved region.
	content := "Claude Code\n\n╭───╮\n│ >\u00a0│\n╰───╯\n"

	normalized := normalizeContent(content)
	assert.NotContains(t, normalized, "\u00a0")

	r := computeRegions(normalized)
	require.True(t, r.hasPromptBox())
	assert.NotContains(t, r.promptBoxBody(), "\u00a0")
	assert.Equal(t, ">", r.promptBoxBody())
}

func TestComputeRegions_RuleDelimitedBarePromptBox(t *testing.T) {
	// Regression (found live 2026-08-04): modern Claude Code UIs render no
	// ╭│╰ box at all — just a horizontal rule, a bare "❯" prompt line (with
	// a trailing NBSP), and another rule, with the footer/status block
	// (cwd/branch/context/model) below the bottom rule. Before this fix,
	// detectPromptBox found no box, abovePromptBox fell back to the last
	// contiguous block — the footer — and the spinner line one block up
	// never entered rule scope, so working was never detected (100% idle
	// misclassification over 131 live polls).
	content := readRegionFixture(t, "bare-prompt-live.txt")
	r := computeRegions(normalizeContent(content))

	require.True(t, r.hasPromptBox(), "a rule-delimited bare ❯ prompt line must be detected as a prompt box")
	assert.Empty(t, r.promptBoxBody(), "a bare ❯ prompt line carries no typed text")
	assert.Contains(t, r.abovePromptBox(), "Swooping",
		"abovePromptBox must reach the spinner line above the top rule, not stop at the box-less fallback")
	assert.NotContains(t, r.abovePromptBox(), "example-repo",
		"the footer block below the bottom rule must be excluded from abovePromptBox")
	assert.NotContains(t, r.abovePromptBox(), "should stay out of scope",
		"scrollback above the blank-line boundary must stay out of abovePromptBox even with the new box shape")
}

func TestDumpRegions_MatchesEngineNormalization(t *testing.T) {
	content := readRegionFixture(t, "basic-box.txt")
	dump := DumpRegions(content)

	const wholeViewport = "Claude Code\n\nPrevious turn output here.\n\n╭──╮\n│ > │\n╰──╯"
	assert.Equal(t, "Previous turn output here.", dump.AboveBox)
	assert.Equal(t, ">", dump.PromptBoxBody)
	// diagnosticBottomLines (15) exceeds this fixture's 7 lines, so BottomLines
	// and AfterLastRule both fall back to the whole viewport here.
	assert.Equal(t, wholeViewport, dump.BottomLines)
	assert.Equal(t, wholeViewport, dump.AfterLastRule)
}
