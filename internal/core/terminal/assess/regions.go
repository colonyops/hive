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

// detectPromptBox finds the last prompt-box shape in lines, trying the
// box-drawn (╭│╰) shape first and falling back to the rule-delimited bare
// prompt shape newer Claude Code UIs render instead (see
// detectRulePromptBox). Both shapes are reported through the same
// promptBoxRegion so every other region method and every rule that consumes
// hasPromptBox/promptBoxBody/abovePromptBox works unchanged regardless of
// which UI rendered the prompt.
func detectPromptBox(lines []string) *promptBoxRegion {
	if box := detectBorderedPromptBox(lines); box != nil {
		return box
	}
	return detectRulePromptBox(lines)
}

// detectBorderedPromptBox finds the LAST box-drawn input box in lines that
// isn't Codex's persistent welcome banner (see isWelcomeBannerBox): a ╰
// border, with a contiguous run of │ body lines above it terminated by a ╭
// border. Claude/codex-style TUIs render this box with rounded corners
// (╭ ╰ │), not the bare ─ rule that a naive "horizontal rule" scan would
// assume. Only the bottommost non-banner ╰ is considered — an older box
// further up the screen (e.g. a dismissed dialog, or the banner sitting
// above the real prompt) is scrollback or chrome, not the current box, so a
// banner match doesn't just fail — the scan retries above it, exactly like
// it would need to for a dismissed-dialog box. If the body run is broken by
// a non-│ line, or the ╭ is never found (a clipped pane where the top
// border has scrolled out), detection fails and the caller falls back to
// whole-viewport behavior rather than misparsing a partial box.
func detectBorderedPromptBox(lines []string) *promptBoxRegion {
	searchEnd := len(lines)
	for searchEnd > 0 {
		box := findLastBorderedBox(lines[:searchEnd])
		if box == nil {
			return nil
		}
		if !isWelcomeBannerBox(box.bodyLines) {
			return box
		}
		searchEnd = box.topIndex
	}
	return nil
}

// findLastBorderedBox finds the last box-drawn input box in lines,
// regardless of what it contains — detectBorderedPromptBox is the one that
// additionally excludes Codex's welcome banner. Split out so the
// banner-skip loop can re-run this same bottommost-╰ scan on a truncated
// prefix without duplicating the parsing logic.
func findLastBorderedBox(lines []string) *promptBoxRegion {
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

// isWelcomeBannerBox reports whether a detected ╭│╰ box is Codex's
// persistent welcome banner (title, model, directory) rather than a real
// input box. Codex renders this banner every frame regardless of state —
// idle, working, mid-approval — unlike Claude's box, it is never the
// current prompt. Left undetected, detectBorderedPromptBox would report the
// banner's static "model: …"/"directory: …" lines as promptBoxBody(),
// which are never a placeholder, so typedInputRule would fire "idle" on
// every single frame regardless of actual state (observed live: idle for
// 458 consecutive polls spanning two full working turns). The banner's
// first body line is always Codex's ">_ <title>" glyph; nothing else this
// engine parses renders that combination, and no committed fixture's real
// input box body starts with it either.
func isWelcomeBannerBox(bodyLines []string) bool {
	if len(bodyLines) == 0 {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(bodyLines[0]), ">_ ")
}

// detectRulePromptBox finds the LAST occurrence, near the bottom of the
// viewport, of the rule-delimited bare prompt shape some modern tool UIs
// render instead of a ╭│╰ box: a horizontal rule, exactly one middle line,
// then another horizontal rule — with the footer/status block (cwd,
// branch, context %, model, permission mode) rendered below the bottom
// rule. Two tool dialects of that middle line are both accepted
// (isBoxlessPromptBodyLine): Claude Code renders a bare prompt glyph (❯ or
// >, optionally followed by typed-but-unsubmitted text); pi renders no
// glyph at all, just a blank/whitespace-only row.
//
// Treating this exactly like a box is what fixes the "modern UI defeats
// prompt-box detection" failure class: topIndex is the TOP rule, so
// abovePromptBox's last-contiguous-block scan starts above the whole
// rule/middle-line/rule unit (landing on the spinner/transcript block above
// the blank line that precedes it) instead of falling through to the
// footer below the bottom rule, and bodyLines is the middle line's content
// after the glyph (or "" for the glyph-less shape) — the same
// typed-but-unsubmitted-text signal a ╭│╰ box's body carries.
//
// The "exactly one middle line between two rules" constraint is deliberate,
// not incidental: pi also renders unrelated content (banners, Q&A
// transcript) between rule pairs further up the screen, sometimes with
// several lines between them. Requiring exactly one line is what keeps
// those from being misparsed as the prompt box. Scanning bottom-up and
// returning on the first match makes this the LAST (bottommost) occurrence
// of the shape, mirroring detectBorderedPromptBox's "only the bottommost ╰"
// rule: an older rule/middle-line/rule unit further up the screen is
// scrollback or unrelated chrome, not the current prompt.
func detectRulePromptBox(lines []string) *promptBoxRegion {
	for i := len(lines) - 1; i >= 2; i-- {
		if !isHorizontalRule(lines[i]) {
			continue
		}
		if !isBoxlessPromptBodyLine(lines[i-1]) {
			continue
		}
		if !isHorizontalRule(lines[i-2]) {
			continue
		}
		return &promptBoxRegion{
			topIndex:    i - 2,
			bottomIndex: i,
			bodyLines:   []string{stripPromptGlyphLine(strings.TrimLeft(lines[i-1], " "))},
		}
	}
	return nil
}

// isBoxlessPromptBodyLine reports whether a line is a valid middle line for
// detectRulePromptBox's rule/middle-line/rule shape: either a blank/
// whitespace-only line (pi's glyph-less empty input row) or a line that,
// after left-trimming, begins with a bare prompt glyph (❯ or >, Claude
// Code's shape).
func isBoxlessPromptBodyLine(line string) bool {
	if strings.TrimSpace(line) == "" {
		return true
	}
	trimmed := strings.TrimLeft(line, " ")
	return strings.HasPrefix(trimmed, "❯") || strings.HasPrefix(trimmed, ">")
}

// stripPromptGlyphLine strips the leading prompt glyph (❯ or >) and one
// following space from an already-left-trimmed prompt line, preserving any
// typed-but-unsubmitted text after it. Trailing padding (tmux pads captured
// lines out to pane width) is trimmed too, matching stripBoxBodyLine's
// behavior for the ╭│╰ box body.
func stripPromptGlyphLine(trimmed string) string {
	body := strings.TrimPrefix(trimmed, "❯")
	body = strings.TrimPrefix(body, ">")
	body = strings.TrimPrefix(body, " ")
	return strings.TrimRight(body, " ")
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
