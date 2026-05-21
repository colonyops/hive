package classifier

import (
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"
)

var knownTools = []string{"claude", "gemini", "aider", "codex", "cursor", "opencode", "cline"}

// TitlePatternsFromConfig compiles config regex strings into TitlePatterns.
// Patterns that fail to compile are logged and skipped.
func TitlePatternsFromConfig(patterns []string) []TitlePattern {
	out := make([]TitlePattern, 0, len(patterns))
	for _, pattern := range patterns {
		compiled, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			log.Warn().Err(err).Str("pattern", pattern).Msg("skipping invalid terminal title pattern")
			continue
		}
		out = append(out, TitlePattern{Pattern: compiled, Tool: inferTool(pattern)})
	}
	return out
}

func inferTool(pattern string) string {
	lower := strings.ToLower(pattern)
	if isPiTitlePattern(lower) {
		return "pi"
	}
	for _, tool := range knownTools {
		if strings.Contains(lower, tool) {
			return tool
		}
	}
	return "agent"
}

func isPiTitlePattern(pattern string) bool {
	trimmed := strings.TrimSpace(pattern)
	return trimmed == "pi" || trimmed == "\\bpi\\b" || trimmed == "(?i)\\bpi\\b" || strings.Contains(trimmed, "π")
}
