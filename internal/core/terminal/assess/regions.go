package assess

import "strings"

// SpinnerGlyphs is the single shared spinner-character inventory. Both the
// working spinner-shape rule and Stage 2's churn normalization reference it
// so the two never drift into separate copies.
var SpinnerGlyphs = []rune{
	'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏', // braille dots
	'·', '✳', '✽', '✶', '✻', '✢', // asterisk spinners
	'⏺', '▸', '▹', '○', '●', // presence dots
}

// promptBoxRegion is the last complete box-drawn input box in the viewport
// (the "╭ ... │ ... ╰" input box rendered by claude/codex-style TUIs).
type promptBoxRegion struct {
	topIndex    int // line index of the ╭ border, within regions.lines
	bottomIndex int // line index of the ╰ border, within regions.lines
	bodyLines   []string
}

// regions is the pre-computed set of viewport slices rules match against.
// Computed once per Assess call from normalized content. All fields and
// methods are unexported: rule code is the only intended consumer, and the
// `hive x assess` diagnostic surface goes through DumpRegions instead.
type regions struct {
	full  string   // the normalized content exactly as given
	lines []string // full split into lines, trailing blank padding trimmed
	box   *promptBoxRegion
}

// computeRegions builds regions from already-normalized content (ANSI
// stripped, NBSP replaced). Interior blank lines are preserved — unlike the
// old detector's getLastNonEmptyLines, which dropped them and distorted
// screen structure.
func computeRegions(content string) regions {
	lines := trimTrailingBlank(strings.Split(content, "\n"))
	return regions{
		full:  content,
		lines: lines,
		box:   detectPromptBox(lines),
	}
}

// viewport returns the full normalized capture, unfiltered.
func (r regions) viewport() string {
	return r.full
}

// bottomLines returns the last n lines of the (padding-trimmed) viewport,
// preserving any interior blank lines.
func (r regions) bottomLines(n int) string {
	if n <= 0 || len(r.lines) == 0 {
		return ""
	}
	start := max(0, len(r.lines)-n)
	return strings.Join(r.lines[start:], "\n")
}

// abovePromptBox returns the most recent contiguous block above the prompt
// box: rules only inspect the block of text immediately preceding the box,
// not everything the screen has ever shown, so historical turns separated by
// a blank line cannot be mistaken for the current one. When no prompt box is
// detected (fullscreen TUIs like vim, clipped panes), it falls back to the
// last contiguous block of the whole viewport — the same blank-line boundary
// restriction still applies, so stale scrollback above that boundary stays
// out of rule scope even without a box. Churn detection still works because
// streaming output appends to (and therefore changes) that last block.
func (r regions) abovePromptBox() string {
	candidate := r.lines
	if r.box != nil {
		candidate = r.lines[:r.box.topIndex]
	}
	return strings.Join(lastContiguousBlock(candidate), "\n")
}

// promptBoxBody returns the interior text of the last input box (typed-but-
// unsubmitted input, or a permission/question dialog rendered in the same
// box), or "" if no box was detected.
func (r regions) promptBoxBody() string {
	if r.box == nil {
		return ""
	}
	return strings.Join(r.box.bodyLines, "\n")
}

// hasPromptBox reports whether a prompt box was detected. Idle rules that key
// off an empty or typed prompt box must not fire when there is no box at all
// (promptBoxBody would otherwise indistinguishably return "" for both cases).
func (r regions) hasPromptBox() bool {
	return r.box != nil
}

// afterLastRule returns content after the last bare horizontal rule
// ("────…"). Falls back to the full viewport when no rule line is found, the
// same graceful-degradation behavior as abovePromptBox.
func (r regions) afterLastRule() string {
	for i := len(r.lines) - 1; i >= 0; i-- {
		if isHorizontalRule(r.lines[i]) {
			return strings.Join(r.lines[i+1:], "\n")
		}
	}
	return strings.Join(r.lines, "\n")
}

func isHorizontalRule(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	for _, r := range trimmed {
		if r != '─' {
			return false
		}
	}
	return true
}

// trimTrailingBlank drops trailing blank lines (tmux pads a captured pane out
// to its full height with empty lines). Interior blanks are untouched.
func trimTrailingBlank(lines []string) []string {
	end := len(lines)
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return lines[:end]
}

// lastContiguousBlock returns the trailing run of non-blank lines, i.e. the
// screen content since the last blank-line boundary.
func lastContiguousBlock(lines []string) []string {
	end := len(lines)
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	start := end
	for start > 0 && strings.TrimSpace(lines[start-1]) != "" {
		start--
	}
	return lines[start:end]
}

// detectPromptBox finds the LAST box-drawn input box in lines: a ╰ border,
// with a contiguous run of │ body lines above it terminated by a ╭ border.
// Claude/codex-style TUIs render this box with rounded corners
// (╭ ╰ │), not the bare ─ rule that a naive "horizontal rule" scan would
// assume. Only the bottommost ╰ is considered — an older box further up the
// screen (e.g. a dismissed dialog) is scrollback, not the current box. If the
// body run is broken by a non-│ line, or the ╭ is never found (a clipped pane
// where the top border has scrolled out), detection fails and the caller
// falls back to whole-viewport behavior rather than misparsing a partial box.
func detectPromptBox(lines []string) *promptBoxRegion {
	bottom := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(strings.TrimLeft(lines[i], " "), "╰") {
			bottom = i
			break
		}
	}
	if bottom < 0 {
		return nil
	}

	top := -1
	var body []string
	for i := bottom - 1; i >= 0; i-- {
		trimmed := strings.TrimLeft(lines[i], " ")
		switch {
		case strings.HasPrefix(trimmed, "╭"):
			top = i
		case strings.HasPrefix(trimmed, "│"):
			body = append([]string{stripBoxBodyLine(trimmed)}, body...)
			continue
		}
		break
	}
	if top < 0 {
		return nil
	}

	return &promptBoxRegion{topIndex: top, bottomIndex: bottom, bodyLines: body}
}

// stripBoxBodyLine strips one layer of "│ " / " │" border padding from an
// already-left-trimmed box body line, preserving interior content (including
// a wholly blank interior line, which becomes "").
func stripBoxBodyLine(trimmed string) string {
	body := strings.TrimPrefix(trimmed, "│")
	body = strings.TrimPrefix(body, " ")
	if idx := strings.LastIndex(body, "│"); idx >= 0 {
		body = body[:idx]
	}
	return strings.TrimRight(body, " ")
}

// RegionDump is the diagnostic view of region extraction for the
// `hive x assess` tooling. Rule sets use the unexported regions type; this
// exists solely so the command surface has something to report.
type RegionDump struct {
	AboveBox      string
	PromptBoxBody string
	BottomLines   string
	AfterLastRule string
}

// diagnosticBottomLines is the window size used for RegionDump.BottomLines —
// generous enough for a human inspecting a capture, distinct from the
// smaller windows individual rules use internally.
const diagnosticBottomLines = 15

// DumpRegions normalizes content the same way Assess does (StripANSI + NBSP)
// so the dump matches what rules actually saw, then resolves every region.
func DumpRegions(content string) RegionDump {
	r := computeRegions(normalizeContent(content))
	return RegionDump{
		AboveBox:      r.abovePromptBox(),
		PromptBoxBody: r.promptBoxBody(),
		BottomLines:   r.bottomLines(diagnosticBottomLines),
		AfterLastRule: r.afterLastRule(),
	}
}
