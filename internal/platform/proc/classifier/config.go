package classifier

import (
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"
)

// TitlePatternsFromConfig compiles config regex strings into TitlePatterns.
// agentNames is the list of known tool names used to infer the tool label
// from each pattern string (e.g. a pattern containing "claude" maps to tool
// "claude"). Pass nil to fall back to the generic "agent" label for all
// patterns. The list should come from config so no source-code change is
// required when a new tool is added.
//
// ToolNamesFromPatterns extracts clean binary names for process detection from
// the same pattern list. Use it alongside TitlePatternsFromConfig so that
// patterns containing regex metacharacters (e.g. "^pi$") map correctly to
// process names (e.g. "pi").
func ToolNamesFromPatterns(patterns []string) []string {
	seen := make(map[string]bool, len(patterns))
	out := make([]string, 0, len(patterns))
	for _, p := range patterns {
		name := cleanPatternName(p)
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

// cleanPatternName strips common regex metacharacters from a pattern to
// produce a plain binary name suitable for process detection.
// Returns empty string if the pattern is too complex to reduce to a name.
func cleanPatternName(pattern string) string {
	r := strings.NewReplacer("^", "", "$", "", "(?i)", "", "\\b", "", "\\B", "")
	name := strings.TrimSpace(r.Replace(strings.ToLower(pattern)))
	// Accept only simple words — letters, digits, hyphens, underscores.
	// Patterns with remaining metacharacters (brackets, dots, stars, etc.)
	// cannot be safely reduced to a single binary name.
	for _, c := range name {
		if c != '-' && c != '_' && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return ""
		}
	}
	return name
}

func TitlePatternsFromConfig(patterns []string, agentNames []string) []TitlePattern {
	out := make([]TitlePattern, 0, len(patterns))
	for _, pattern := range patterns {
		compiled, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			log.Warn().Err(err).Str("pattern", pattern).Msg("skipping invalid terminal title pattern")
			continue
		}
		out = append(out, TitlePattern{Pattern: compiled, Tool: inferTool(pattern, agentNames)})
	}
	return out
}

func inferTool(pattern string, agentNames []string) string {
	lower := strings.ToLower(pattern)
	if isPiTitlePattern(lower) {
		return "pi"
	}
	for _, name := range agentNames {
		if strings.Contains(lower, name) {
			return name
		}
	}
	return "agent"
}

func isPiTitlePattern(pattern string) bool {
	if strings.Contains(pattern, "π") {
		return true
	}
	// Match any anchored or word-boundary variant that reduces to "pi".
	return cleanPatternName(pattern) == "pi"
}
