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
	trimmed := strings.TrimSpace(pattern)
	return trimmed == "pi" || trimmed == "\\bpi\\b" || trimmed == "(?i)\\bpi\\b" || strings.Contains(trimmed, "π")
}
