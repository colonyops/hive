package status

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/core/terminal/assess"
)

// The patterns below mirror internal/core/terminal/state_tracker.go's
// NormalizeContent. They are copied rather than shared because Phase 4
// deletes the old pipeline's copies outright; factoring out a common helper
// for code on its way out isn't worth the coupling.
var (
	dynamicStatusPattern   = regexp.MustCompile(`\([^)]*\d+s\s*·[^)]*(?:tokens|↑|↓)[^)]*\)`)
	progressBarPattern     = regexp.MustCompile(`\[=*>?\s*\]\s*\d+%`)
	timePattern            = regexp.MustCompile(`\b\d{1,2}:\d{2}(:\d{2})?\b`)
	percentagePattern      = regexp.MustCompile(`\b\d{1,3}%`)
	downloadPattern        = regexp.MustCompile(`\d+(\.\d+)?[KMGT]?B/\d+(\.\d+)?[KMGT]?B`)
	blankLinesPattern      = regexp.MustCompile(`\n{3,}`)
	statusLinePattern      = regexp.MustCompile(`📂[^•]+•[^•]+•[^•\n]+`)
	gitBranchStatusPattern = regexp.MustCompile(`🌿\s*[a-zA-Z0-9/_-]+`)

	// thinkingPatternEllipsis builds its spinner-glyph character class from
	// assess.SpinnerGlyphs instead of a private copy, so churn normalization
	// tracks whatever glyphs Stage 1 rules recognize rather than drifting
	// into a second glyph inventory.
	thinkingPatternEllipsis = regexp.MustCompile(spinnerGlyphClass() + `\s*.+…\s*\([^)]*\)`)
)

func spinnerGlyphClass() string {
	var b strings.Builder
	b.WriteByte('[')
	for _, r := range assess.SpinnerGlyphs {
		b.WriteRune(r)
	}
	b.WriteByte(']')
	return b.String()
}

// normalizeContent strips dynamic, animation-driven substrings (spinner
// glyphs, running counters, timestamps, status lines) so churn hashing
// reacts to real content changes, not per-frame cosmetic noise.
func normalizeContent(content string) string {
	result := terminal.StripANSI(content)
	result = stripControlChars(result)

	for _, r := range assess.SpinnerGlyphs {
		result = strings.ReplaceAll(result, string(r), "")
	}

	result = dynamicStatusPattern.ReplaceAllString(result, "(STATUS)")
	result = thinkingPatternEllipsis.ReplaceAllString(result, "THINKING…")
	result = progressBarPattern.ReplaceAllString(result, "[PROGRESS]")
	result = downloadPattern.ReplaceAllString(result, "X.XMB/Y.YMB")
	result = percentagePattern.ReplaceAllString(result, "N%")
	result = timePattern.ReplaceAllString(result, "HH:MM:SS")
	result = statusLinePattern.ReplaceAllString(result, "[STATUSLINE]")
	result = gitBranchStatusPattern.ReplaceAllString(result, "[BRANCH]")

	lines := strings.Split(result, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	result = strings.Join(lines, "\n")

	result = blankLinesPattern.ReplaceAllString(result, "\n\n")

	return result
}

// stripControlChars removes ASCII control characters except tab, newline, CR.
func stripControlChars(content string) string {
	var result strings.Builder
	result.Grow(len(content))
	for _, r := range content {
		if (r >= 32 && r != 127) || r == '\t' || r == '\n' || r == '\r' {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// hashContent returns a stable hex digest of content, used to detect churn
// between polls. Normalized (not raw) content is hashed, since spinner
// animation never stabilizes raw bytes — see normalizeContent.
func hashContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
